package controllers

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/revel/revel"
	logger "github.com/sirupsen/logrus"
)

func (c App) Download() revel.Result {
	sourcePath := c.Params.Get("source_path")
	localPath := c.Params.Get("local_path")
	isDir := c.Params.Get("is_dir")
	fileNamePost := c.Params.Get("file_name")
	fileName := filepath.Base(fileNamePost)

	if connection := c.EstablishSSHConnection(); connection != nil {
		return connection
	}

	defer SSHclient.Close()

	if dirOrNot, err := strconv.ParseBool(isDir); err != nil {
		logger.Warnf("We don't understand if it's a file or directory: %v", err)
		response := CompileJSONResult(false, "We don't understand if it's a file or directory")
		return c.RenderJSON(response)
	} else {
		if dirOrNot {
			stdoutNewPipe, err := SSHsession.StdoutPipe()
			if err != nil {
				logger.Warnf("Cannot create a pipe to download folder: %v", err)
				response := CompileJSONResult(false, "Cannot create a pipe to download folder")
				return c.RenderJSON(response)
			}
			tempArchiveName := fmt.Sprintf("%sdownload_%v.tar.gz", localPath+string(filepath.Separator), time.Now().UnixNano())
			tempArchiveFile, err := os.OpenFile(tempArchiveName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
			if err != nil {
				logger.Warnf("Cannot open temp archive to write to: %v", err)
				response := CompileJSONResult(false, "Cannot open temp archive to write to")
				return c.RenderJSON(response)
			}
			if err := SSHsession.Start("tar cz --directory='" + sourcePath + "' '" + fileName + "'"); err != nil {
				logger.Warnf("Problem with running command via SSH: %v", err)
				response := CompileJSONResult(false, "Problem with running command via SSH")
				return c.RenderJSON(response)
			}
			_, err = io.Copy(tempArchiveFile, stdoutNewPipe)
			if err != nil {
				logger.Warnf("Problem with copying archive via SSH: %v", err)
				response := CompileJSONResult(false, "Problem with copying archive via SSH")
				return c.RenderJSON(response)
			}

			//if err := SSHsession.Wait(); err != nil {
			//	logger.Warnf("Something is wrong while waiting for command to complete: %v", err)
			//	response := CompileJSONResult(false, "Something is wrong while waiting for command to complete")
			//	return c.RenderJSON(response)
			//}

			SSHsession.Close()
			tempArchiveFile.Close()

			archiveOpen, err := os.Open(tempArchiveName)
			if err != nil {
				if err := os.Remove(tempArchiveName); err != nil {
					logger.Warnf("Unable to remove temp archive: %v", err)
				}
				logger.Warnf("Problem with opening temp archive: %v", err)
				response := CompileJSONResult(false, "Problem with opening temp archive")
				return c.RenderJSON(response)
			}
			defer func() {
				archiveOpen.Close()
				if err := os.Remove(tempArchiveName); err != nil {
					logger.Warnf("Unable to remove temp archive: %v", err)
				}
			}()

			gzipOpen, err := gzip.NewReader(archiveOpen)
			if err != nil {
				logger.Warnf("Problem with creating stream from temp archive: %v", err)
				response := CompileJSONResult(false, "Problem with creating stream from temp archive")
				return c.RenderJSON(response)
			}

			tarReader := tar.NewReader(gzipOpen)

			for {
				header, err := tarReader.Next()

				if err == io.EOF {
					break
				}
				if err != nil {
					logger.Warnf("Problem with reading temp archive: %v", err)
					response := CompileJSONResult(false, "Problem with reading temp archive")
					return c.RenderJSON(response)
				}

				switch header.Typeflag {
				case tar.TypeDir:
					if err := os.Mkdir(localPath+string(filepath.Separator)+header.Name, 0755); err != nil {
						logger.Warnf("Cannot create a directory from archive: %v", err)
						response := CompileJSONResult(false, "Cannot create a directory from archive")
						return c.RenderJSON(response)
					}
				case tar.TypeReg:
					outFile, err := os.Create(localPath + string(filepath.Separator) + header.Name)
					if err != nil {
						logger.Warnf("Cannot create a file from archive: %v", err)
						response := CompileJSONResult(false, "Cannot create a file from archive")
						return c.RenderJSON(response)
					}

					if _, err := io.Copy(outFile, tarReader); err != nil {
						logger.Warnf("Failed to write to a file from archive: %v", err)
						response := CompileJSONResult(false, "Failed to write to a file from archive")
						return c.RenderJSON(response)
					}
					outFile.Close()
				default:
					logger.Warnf("General archive problem: uknown type: %s in %s", header.Typeflag, header.Name)
					response := CompileJSONResult(false, "General archive problem, Refer to logs :(")
					return c.RenderJSON(response)
				}
			}
		} else {
			remoteFileContent, err := SSHsession.Output("cat '" + fileNamePost + "'")
			if err != nil {
				logger.Warnf("Problem with getting file content: %v", err)
				response := CompileJSONResult(false, "Problem with getting file content")
				return c.RenderJSON(response)
			}
			if err := ioutil.WriteFile(localPath+string(filepath.Separator)+fileName, remoteFileContent, 644); err != nil {
				logger.Warnf("Problem with writing file content: %v", err)
				response := CompileJSONResult(false, "Problem with writing file content")
				return c.RenderJSON(response)
			}
		}
	}
	response := CompileJSONResult(true, "")
	return c.RenderJSON(response)
}