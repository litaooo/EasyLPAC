package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"fyne.io/fyne/v2/dialog"
	"github.com/mattn/go-runewidth"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func runLpac(args []string) (json.RawMessage, error) {
	StatusChan <- StatusProcess
	LockButtonChan <- true
	defer func() {
		StatusChan <- StatusReady
		LockButtonChan <- false
	}()

	// Save to LogFile
	lpacPath := filepath.Join(ConfigInstance.LpacDir, ConfigInstance.EXEName)
	command := lpacPath
	for _, arg := range args {
		command += fmt.Sprintf(" %s", arg)
	}
	if _, err := fmt.Fprintln(ConfigInstance.LogFile, command); err != nil {
		return nil, err
	}

	cmd := exec.Command(lpacPath, args...)
	HideCmdWindow(cmd)

	cmd.Dir = ConfigInstance.LpacDir

	cmd.Env = []string{
		fmt.Sprintf("APDU_INTERFACE=%s", ConfigInstance.APDUInterface),
		fmt.Sprintf("HTTP_INTERFACE=%s", ConfigInstance.HTTPInterface),
		fmt.Sprintf("DRIVER_IFID=%s", ConfigInstance.DriverIFID),
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	writer := io.MultiWriter(&stdout, ConfigInstance.LogFile)
	errWriter := io.MultiWriter(ConfigInstance.LogFile, &stderr)
	cmd.Stdout = writer
	cmd.Stderr = errWriter

	err := cmd.Run()
	if err != nil {
		if len(bytes.TrimSpace(stderr.Bytes())) != 0 {
			return nil, fmt.Errorf("%s", stderr.String())
		}
	}

	var resp LpacReturnValue

	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			return nil, err
		}
		if resp.Payload.Code != 0 {
			var dataString string
			// 外层
			var jsonString string
			_ = json.Unmarshal(resp.Payload.Data, &jsonString)
			// 内层
			var result map[string]interface{}
			err = json.Unmarshal([]byte(jsonString), &result)
			if err != nil {
				dataString = jsonString
			} else {
				formattedJSON, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					dataString = jsonString
				} else {
					dataString = string(formattedJSON)
				}
			}
			wrapText := func(text string, maxWidth int) string {
				var result strings.Builder
				lines := strings.Split(text, "\n")
				for _, line := range lines {
					var currentWidth int
					var currentLine strings.Builder
					for _, runeValue := range line {
						// 使用字符宽度而不是长度，让包含 CJK 字符的字符串也能正确限制显示长度
						runeWidth := runewidth.RuneWidth(runeValue)
						if currentWidth+runeWidth > maxWidth {
							result.WriteString(currentLine.String() + "\n")
							currentLine.Reset()
							currentWidth = 0
						}
						currentLine.WriteRune(runeValue)
						currentWidth += runeWidth
					}
					if currentLine.Len() > 0 {
						result.WriteString(currentLine.String() + "\n")
					}
				}
				return result.String()
			}
			return nil, fmt.Errorf("stage: %s\ndata: %s", resp.Payload.Message, wrapText(dataString, 90))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return resp.Payload.Data, nil
}

func LpacChipInfo() (EuiccInfo, error) {
	args := []string{"chip", "info"}
	payload, err := runLpac(args)
	if err != nil {
		return EuiccInfo{}, err
	}
	var chipInfo EuiccInfo
	if err = json.Unmarshal(payload, &chipInfo); err != nil {
		return EuiccInfo{}, err
	}
	return chipInfo, nil
}

func LpacProfileList() ([]Profile, error) {
	args := []string{"profile", "list"}
	payload, err := runLpac(args)
	if err != nil {
		return nil, err
	}
	var profiles []Profile
	if err = json.Unmarshal(payload, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func LpacProfileEnable(iccid string) error {
	args := []string{"profile", "enable", iccid}
	_, err := runLpac(args)
	if err != nil {
		return err
	}
	return nil
}

func LpacProfileDisable(iccid string) error {
	args := []string{"profile", "disable", iccid}
	_, err := runLpac(args)
	if err != nil {
		return err
	}
	return nil
}

func LpacProfileDelete(iccid string) error {
	args := []string{"profile", "delete", iccid}
	_, err := runLpac(args)
	if err != nil {
		return err
	}
	return nil
}

func LpacProfileDownload(info PullInfo) {
	args := []string{"profile", "download"}
	if info.SMDP != "" {
		args = append(args, "-s", info.SMDP)
	}
	if info.MatchID != "" {
		args = append(args, "-m", info.MatchID)
	}
	if info.ConfirmCode != "" {
		args = append(args, "-c", info.ConfirmCode)
	}
	if info.IMEI != "" {
		args = append(args, "-i", info.IMEI)
	}
	_, err := runLpac(args)
	if err != nil {
		ShowLpacErrDialog(err)
	} else {
		notificationOrigin := Notifications
		Refresh()
		d := dialog.NewConfirm("Send Install Notification",
			"Download successful\nSend the install notification now?\n",
			func(b bool) {
				if b {
					downloadNotification := findNewNotification(notificationOrigin, Notifications)
					go processNotification(downloadNotification.SeqNumber)
				}
			}, WMain)
		d.Show()
	}
}

func LpacProfileDiscovery() ([]DiscoveryResult, error) {
	args := []string{"profile", "discovery"}
	payload, err := runLpac(args)
	if err != nil {
		return nil, err
	}
	var data []DiscoveryResult
	if err = json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func LpacProfileNickname(iccid, nickname string) error {
	args := []string{"profile", "nickname", iccid, nickname}
	_, err := runLpac(args)
	if err != nil {
		return err
	}
	return nil
}

func LpacNotificationList() ([]Notification, error) {
	args := []string{"notification", "list"}
	payload, err := runLpac(args)
	if err != nil {
		return nil, err
	}
	var notifications []Notification
	if err = json.Unmarshal(payload, &notifications); err != nil {
		return nil, err
	}
	return notifications, nil
}

func LpacNotificationProcess(seq int) error {
	args := []string{"notification", "process", strconv.Itoa(seq)}
	_, err := runLpac(args)
	if err != nil {
		return err
	}
	return nil
}

func LpacNotificationRemove(seq int) error {
	args := []string{"notification", "remove", strconv.Itoa(seq)}
	_, err := runLpac(args)
	if err != nil {
		return err
	}
	return nil
}

func LpacDriverApduList() ([]ApduDriver, error) {
	args := []string{"driver", "apdu", "list"}
	payload, err := runLpac(args)
	if err != nil {
		return nil, err
	}
	var apduDrivers []ApduDriver
	if err = json.Unmarshal(payload, &apduDrivers); err != nil {
		return nil, err
	}
	return apduDrivers, nil
}

func LpacChipDefaultSmdp(smdp string) error {
	args := []string{"chip", "defaultsmdp", smdp}
	_, err := runLpac(args)
	if err != nil {
		return err
	}
	return nil
}
