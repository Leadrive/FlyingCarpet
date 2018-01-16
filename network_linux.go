package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func connectToPeer(t *Transfer) bool {
	if Mode == "sending" {
		if !checkForFile(t) {
			t.output(fmt.Sprintf("Could not find file to send: %s", t.Filepath))
			return false
		}
		if t.Peer == "mac" {
			if !startAdHoc(t) {
				return false
			}
			var ok bool
			t.RecipientIP, ok = findMac(t)
			if !ok {
				return false
			}
		} else if t.Peer == "windows" {
			if !joinAdHoc(t) {
				return false
			}
			t.RecipientIP = findWindows(t)
		} else if t.Peer == "linux" {
			if !joinAdHoc(t) {
				return false
			}
			var ok bool
			t.RecipientIP, ok = findLinux(t)
			if !ok {
				return false
			}
		}
	} else if Mode == "receiving" {
		if t.Peer == "windows" {
			if !joinAdHoc(t) {
				return false
			}
			// go stayOnAdHoc(t)
		} else if t.Peer == "mac" {
			if !startAdHoc(t) {
				return false
			}
		} else if t.Peer == "linux" {
			if !startAdHoc(t) {
				return false
			}
		}
	}
	return true
}

func startAdHoc(t *Transfer) bool {
	// or just:
	// nmcli dev wifi hotspot ssid t.SSID band bg channel 11 password t.Passphrase + t.Passphrase
	// ??
	commands := []string{"nmcli con add type wifi ifname " + getWifiInterface() + " con-name " + t.SSID + " autoconnect yes ssid " + t.SSID,
		"nmcli con modify " + t.SSID + " 802-11-wireless.mode ap 802-11-wireless.band bg ipv4.method shared",
		"nmcli con modify " + t.SSID + " wifi-sec.key-mgmt wpa-psk",
		"nmcli con modify " + t.SSID + " wifi-sec.psk \"" + t.Passphrase + t.Passphrase + "\"",
		"nmcli con up " + t.SSID}
	for _, cmd := range commands {
		out := runCommand(cmd)
		if out != "" {
			t.output(out)
		}
	}
	return true
}

func stopAdHoc(t *Transfer) {
	command := "nmcli con down " + t.SSID
	t.output(runCommand(command))
}

func joinAdHoc(t *Transfer) bool {
	t.output("Looking for ad-hoc network " + t.SSID + " for " + strconv.Itoa(JOIN_ADHOC_TIMEOUT) + " seconds...")
	timeout := JOIN_ADHOC_TIMEOUT
	var outBytes []byte
	err := errors.New("")
	commands := []string{"nmcli con add type wifi ifname " + getWifiInterface() + " con-name \"" + t.SSID + "\" autoconnect yes ssid \"" + t.SSID + "\"",
		"nmcli con modify \"" + t.SSID + "\" wifi-sec.key-mgmt wpa-psk",
		"nmcli con modify \"" + t.SSID + "\" wifi-sec.psk \"" + t.Passphrase + t.Passphrase + "\"",
		"nmcli con up \"" + t.SSID + "\""}
	for i, cmd := range commands {
		outBytes, err = exec.Command("sh", "-c", cmd).CombinedOutput()
		// t.output(fmt.Sprintf("outBytes %d: %s", i, string(outBytes)))
		if err != nil {
			t.output(fmt.Sprintf("Error %d: %s", i, err.Error()))
		}
	}
	for string(outBytes)[:5] == "Error" {
		if timeout <= 0 {
			t.output("Could not find the ad hoc network within " + strconv.Itoa(JOIN_ADHOC_TIMEOUT) + " seconds.")
			return false
		}
		timeout -= 5
		time.Sleep(time.Second * time.Duration(5))
		outBytes, err = exec.Command("sh", "-c", "nmcli con up \""+t.SSID+"\"").CombinedOutput()
		t.output(string(outBytes))
		if err != nil {
			t.output(fmt.Sprintf("Error joining ad hoc network: %s", err))
		}
	}
	t.output(string(outBytes))
	return true
}

func resetWifi(t *Transfer) {
	command := "nmcli con down \"" + t.SSID + "\""
	runCommand(command)

	if Mode == "sending" || t.Peer == "windows" {
		// To delete all FC SSIDs:
		// nmcli -t -f name con | grep flyingCarpet* | xargs -d '\n' nmcli con delete
		command := "nmcli con delete " + t.SSID
		t.output(runCommand(command))
	}

	command = "nmcli con up " + PreviousSSID
	runCommand(command)

	return
}

func getCurrentWifi(t *Transfer) (ssid string) {
	command := "nmcli -f active,ssid dev wifi | awk '/^yes/{print $2}"
	ssid = runCommand(command)
	return
}

func getCurrentUUID(t *Transfer) (uuid string) {
	command := "nmcli -f active,uuid con | awk '/^yes/{print $2}'"
	uuid = runCommand(command)
	return
}

func getWifiInterface() (iface string) {
	command := "ifconfig | awk '/^wl/{print $1}'"
	iface = runCommand(command)
	return
}

func getIPAddress(t *Transfer) (ip string) {
	command := "ifconfig wlp2s0 | awk '{print $2}' | grep -oP 'addr:\\K.*'"
	ip = runCommand(command)
	return
}

func findMac(t *Transfer) (peerIP string, success bool) {
	timeout := FIND_MAC_TIMEOUT
	currentIP := getIPAddress(t)
	pingString := "ping -b -c 5 $(ifconfig | awk '/Bcast/ {print substr($3,7)}') 2>&1 | " + // ping broadcast address, include stderr
		"grep --line-buffered -oE '[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}' | " + // get all IPs
		"grep --line-buffered -vE $(ifconfig | awk '/Bcast/ {print substr($3,7)}') | " + // exclude broadcast address
		"grep -vE '" + currentIP + "'" // exclude current IP

	t.output("Looking for peer IP for " + strconv.Itoa(FIND_MAC_TIMEOUT) + " seconds.")
	for peerIP == "" {
		if timeout <= 0 {
			t.output("Could not find the peer computer within " + strconv.Itoa(FIND_MAC_TIMEOUT) + " seconds.")
			return "", false
		}
		pingBytes, pingErr := exec.Command("sh", "-c", pingString).CombinedOutput()
		if pingErr != nil {
			// t.output(fmt.Sprintf("Could not find peer. Waiting %2d more seconds. %s", timeout, pingErr))
			timeout -= 2
			time.Sleep(time.Second * time.Duration(2))
			continue
		}
		peerIPs := string(pingBytes)
		peerIP = peerIPs[:strings.Index(peerIPs, "\n")]
	}
	t.output(fmt.Sprintf("Peer IP found: %s", peerIP))
	success = true
	return
}

func findWindows(t *Transfer) (peerIP string) {
	currentIP := getIPAddress(t)
	if strings.Contains(currentIP, "192.168.137") {
		return "192.168.137.1"
	} else {
		return "192.168.173.1"
	}
}

func findLinux(t *Transfer) (peerIP string, success bool) {
	return "10.42.0.1", true
}

func runCommand(cmd string) (output string) {
	cmdBytes, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(cmdBytes))
}

func checkForFile(t *Transfer) bool {
	_, err := os.Stat(t.Filepath)
	if err != nil {
		return false
	}
	return true
}