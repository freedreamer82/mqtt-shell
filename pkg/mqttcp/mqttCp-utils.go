package mqttcp

import (
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"path"
)

func calculateMD5(fileName string) ([]byte, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	hash := md5.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		return nil, err
	}

	return hash.Sum(nil), nil
}

func takeFileInfo(fileName string) (int64, string, error) {
	info, err := os.Stat(fileName)
	if os.IsNotExist(err) || info.IsDir() {
		return 0, "", errors.New(fmt.Sprintf("%s : not found", fileName))
	} else if err != nil {
		return 0, "", err
	}

	size := info.Size()

	md5Value, errMd5 := calculateMD5(fileName)
	if errMd5 != nil {
		return 0, "", errMd5
	}

	return size, fmt.Sprintf("%x", md5Value), nil
}

func fileDestinationPathCheck(local, remote string) (string, error) {
	newLocal := local

	dir, f := path.Split(newLocal)

	dInfo, errD := os.Stat(dir)
	if os.IsNotExist(errD) {
		return newLocal, errors.New(fmt.Sprintf("%s dir not exist", dir))
	} else if errD == nil && !dInfo.IsDir() {
		return newLocal, errors.New(fmt.Sprintf("%s is not dir", dir))
	}

	if f == "" || f == "." {
		base := path.Base(remote)
		if base != "" {
			newLocal = path.Join(dir, base)
		} else {
			return newLocal, errors.New("file name not valid")
		}
	}

	_, errF := os.Stat(newLocal)
	if errF == nil {
		return newLocal, errors.New(fmt.Sprintf("%s already exist", newLocal))
	}

	return newLocal, nil
}

func decodeData(dataraw []byte) []byte {

	var data = string(dataraw)
	rawdecoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		log.Error("decode error:", err)
		return nil
	}
	return rawdecoded
}
