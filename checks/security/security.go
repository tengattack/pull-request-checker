package security

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// constants
const (
	Golang Language = "golang"

	rikiScan = "https://riki.bilibili.co/api/bvd/package/"
)

// Language language
type Language string

type reqData struct {
	AppName string   `json:"app_name"`
	Lang    Language `json:"lang"`
	Data    string   `json:"data"`
}

var client = http.Client{
	Timeout: 5 * time.Second,
}

// ScanPKG checks the security of the packages listed in pkgFile, such as "go.sum"
func ScanPKG(lang Language, projectName, repoPath, pkgFile string) (bool, error) {
	fileContent, err := ioutil.ReadFile(filepath.Join(repoPath, pkgFile))
	if err != nil {
		return false, err
	}

	data := reqData{
		AppName: projectName,
		Lang:    lang,
		Data:    strings.ReplaceAll(base64.StdEncoding.EncodeToString(fileContent), "\n", ""),
	}
	body, err := json.Marshal(data)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest(http.MethodPost, rikiScan, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	return false, nil
}
