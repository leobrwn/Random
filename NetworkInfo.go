package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	destination = "/tmp/temp/data"
	outputName  = "network-map.txt"
)

var (
	scanRadio bool
	out       strings.Builder
)

var webPorts = map[string]bool{
	"80": true, "443": true, "3000": true, "3001": true, "4000": true,
	"5000": true, "5173": true, "8000": true, "8008": true, "8080": true,
	"8081": true, "8443": true, "8888": true,
}


func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runCmd(name string, args ...string) string {
	if !commandExists(name) {
		return ""
	}
	output, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(output), "\n")
}

func isActive(service string) bool {
	return exec.Command("systemctl", "is-active", "--quiet", service).Run() == nil
}


func field(label, value string) {
	if value == "" {
		value = "N/A"
	}
	value = strings.ReplaceAll(value, "\n", " ")
	fmt.Fprintf(&out, "%-24s %s\n", label, value)
}

func details(label, value string) {
	fmt.Fprintf(&out, "%s\n", label)
	if value == "" {
		fmt.Fprint(&out, "  N/A\n")
		return
	}
	for _, line := range strings.Split(value, "\n") {
		fmt.Fprintf(&out, "  %s\n", line)
	}
}

func section(title string) {
	fmt.Fprintf(&out, "\n--- %s ---\n", title)
}



func getDefaultRouteLine() []string {
	lines := strings.Split(runCmd("ip", "route", "show", "default"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return nil
	}
	return strings.Fields(lines[0])
}

func getDefaultRoute() string {
	fields := getDefaultRouteLine()
	if fields == nil {
		return ""
	}
	return strings.Join(fields, " ")
}

func getDefaultInterface() string {
	fields := getDefaultRouteLine()
	if len(fields) < 5 {
		return ""
	}
	return fields[4]
}

func getLocalIP() string {
	for _, line := range strings.Split(runCmd("ip", "-4", "route", "get", "1.1.1.1"), "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "src" && i+1 < len(fields) {
				return fields[i+1]
			}
		}
	}
	return ""
}

func getGlobalIPv6() string {
	var addrs []string
	for _, line := range strings.Split(runCmd("ip", "-6", "addr", "show", "scope", "global"), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "inet6" {
			addrs = append(addrs, fields[1])
		}
	}
	return strings.Join(addrs, ", ")
}

func getPublicIP() string {
	return runCmd("curl", "-fsS", "--max-time", "5", "https://api.ipify.org")
}

func listInterfaces() string {
	return runCmd("ip", "-brief", "address")
}

func listMACAddresses() string {
	var lines []string
	for _, line := range strings.Split(runCmd("ip", "-o", "link", "show"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		iface := strings.TrimSuffix(fields[1], ":")
		for i, f := range fields {
			if f == "link/ether" && i+1 < len(fields) {
				lines = append(lines, iface+": "+fields[i+1])
			}
		}
	}
	return strings.Join(lines, "\n")
}

func listRoutes() string    { return runCmd("ip", "route", "show") }
func listNeighbors() string { return runCmd("ip", "neigh", "show") }

func listActiveConnections() string {
	return runCmd("nmcli", "-f", "NAME,TYPE,DEVICE", "connection", "show", "--active")
}


func readResolvConf() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	return strings.Split(string(data), "\n")
}

func getDNSServers() string {
	var servers []string
	for _, line := range readResolvConf() {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			servers = append(servers, fields[1])
		}
	}
	return strings.Join(servers, ", ")
}

func getSearchDomains() string {
	var domains []string
	for _, line := range readResolvConf() {
		fields := strings.Fields(line)
		if len(fields) >= 2 && (fields[0] == "search" || fields[0] == "domain") {
			domains = append(domains, fields[1:]...)
		}
	}
	return strings.Join(domains, ", ")
}

func listListeningTCP() string { return runCmd("ss", "-ltnp") }
func listListeningUDP() string { return runCmd("ss", "-lunp") }

func listRunningServices() string {
	var services []string
	output := runCmd("systemctl", "list-units", "--type=service", "--state=running", "--no-legend", "--no-pager")
	for _, line := range strings.Split(output, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			services = append(services, fields[0])
		}
	}
	return strings.Join(services, "\n")
}

func getSSHStatus() string {
	var lines []string

	for _, line := range strings.Split(runCmd("ss", "-ltnH"), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && strings.HasSuffix(fields[3], ":22") {
			lines = append(lines, "Listening on "+fields[3])
		}
	}

	if commandExists("systemctl") {
		if isActive("sshd") {
			lines = append(lines, "sshd service is active")
		} else if isActive("ssh") {
			lines = append(lines, "ssh service is active")
		}
	}

	return strings.Join(lines, "\n")
}

func getSSHConfiguration() string {
	data, err := os.ReadFile("/etc/ssh/sshd_config")
	if err != nil {
		return ""
	}
	keys := []string{"Port", "PermitRootLogin", "PasswordAuthentication"}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		for _, key := range keys {
			if strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"\t") {
				lines = append(lines, trimmed)
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

func getFirewallStatus() string {
	var lines []string

	if commandExists("ufw") {
		if s := runCmd("ufw", "status"); s != "" {
			lines = append(lines, s)
		}
	}
	if commandExists("firewall-cmd") {
		lines = append(lines, "firewalld: "+runCmd("firewall-cmd", "--state"))
	}
	if commandExists("systemctl") && isActive("nftables") {
		lines = append(lines, "nftables service: active")
	}

	return strings.Join(lines, "\n")
}

func listLocalWebPorts() string {
	var lines []string
	for _, line := range strings.Split(runCmd("ss", "-ltnH"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		address := fields[3]
		idx := strings.LastIndex(address, ":")
		if idx == -1 {
			continue
		}
		if webPorts[address[idx+1:]] {
			lines = append(lines, address+" (possible HTTP/API service)")
		}
	}
	return strings.Join(lines, "\n")
}


func getWifiState() string { return runCmd("nmcli", "radio", "wifi") }

func getCurrentWifi() string {
	for _, line := range strings.Split(runCmd("nmcli", "-t", "-f", "ACTIVE,SSID", "dev", "wifi"), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && parts[0] == "yes" {
			return parts[1]
		}
	}
	return ""
}

func listWifiNetworks() string {
	if !commandExists("nmcli") {
		return ""
	}
	if scanRadio {
		exec.Command("nmcli", "device", "wifi", "rescan").Run()
	}
	return runCmd("nmcli", "-f", "IN-USE,SSID,BSSID,SIGNAL,SECURITY", "device", "wifi", "list", "--rescan", "no")
}

func getBluetoothController() string {
	var lines []string
	for _, line := range strings.Split(runCmd("bluetoothctl", "show"), "\n") {
		if strings.Contains(line, "Controller") || strings.Contains(line, "Powered:") ||
			strings.Contains(line, "Discovering:") || strings.Contains(line, "Pairable:") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func listBluetoothDevices() string {
	if !commandExists("bluetoothctl") {
		return ""
	}
	if scanRadio && commandExists("timeout") {
		exec.Command("timeout", "8", "bluetoothctl", "scan", "on").Run()
		exec.Command("bluetoothctl", "scan", "off").Run()
	}
	return runCmd("bluetoothctl", "devices")
}



func parseArgs() {
	switch len(os.Args) {
	case 1:
		return
	case 2:
		if os.Args[1] == "--scan-radio" {
			scanRadio = true
			return
		}
	}
	os.Exit(1)
}

func writeOutput() {
	os.MkdirAll(destination, 0700)
	os.WriteFile(filepath.Join(destination, outputName), []byte(out.String()), 0600)
}

func getnetinfo() {
	parseArgs()

	fmt.Fprint(&out, "=== NETWORK MAP ===\n")
	field("Collected:", time.Now().Format("2006-01-02 15:04:05 MST"))
	hostname, _ := os.Hostname()
	field("Hostname:", hostname)
	field("Radio discovery:", fmt.Sprintf("%t", scanRadio))

	section("Network Overview")
	field("Default interface:", getDefaultInterface())
	field("Default route:", getDefaultRoute())
	field("Local IPv4:", getLocalIP())
	field("Global IPv6:", getGlobalIPv6())
	field("Public IPv4:", getPublicIP())
	field("DNS servers:", getDNSServers())
	field("Search domains:", getSearchDomains())
	details("Active connections:", listActiveConnections())

	section("Interfaces and Routes")
	details("Interfaces:", listInterfaces())
	details("MAC addresses:", listMACAddresses())
	details("Routing table:", listRoutes())
	details("Known network neighbors:", listNeighbors())

	section("Local Ports and Services")
	details("Listening TCP ports:", listListeningTCP())
	details("Listening UDP ports:", listListeningUDP())
	details("Running system services:", listRunningServices())
	details("SSH status:", getSSHStatus())
	details("SSH configuration:", getSSHConfiguration())
	details("Firewall status:", getFirewallStatus())
	details("Possible local web/API ports:", listLocalWebPorts())

	section("Wireless")
	field("Wi-Fi radio:", getWifiState())
	field("Current Wi-Fi:", getCurrentWifi())
	details("Visible Wi-Fi networks:", listWifiNetworks())
	details("Bluetooth controller:", getBluetoothController())
	details("Known Bluetooth devices:", listBluetoothDevices())

	fmt.Fprint(&out, "\n--- Notes ---\n")
	fmt.Fprint(&out, "  Known neighbors come from the local ARP/NDP cache; this is not an active LAN scan.\n")
	fmt.Fprint(&out, "  Possible web/API ports identify common listening port numbers only; API routes cannot be discovered generically.\n")
	if scanRadio {
		fmt.Fprint(&out, "  Wi-Fi and Bluetooth radio discovery was requested for this report.\n")
	} else {
		fmt.Fprint(&out, "  Wi-Fi uses cached scan results and Bluetooth lists known devices. Run with --scan-radio for active radio discovery.\n")
	}

	writeOutput()
}
