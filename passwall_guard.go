package main

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
	"bufio"
	"golang.org/x/crypto/ssh"
	"net"
	"encoding/json"
)

var Config map[string]interface{}

var openwrtIP string
var basicRestartCmd = "uci commit passwall && /etc/init.d/haproxy restart && /etc/init.d/passwall restart"
var getNewSubCmd = "lua /usr/share/passwall/subscribe.lua start"
var nodes []string


func myExternalIPv4() string {
	// Create a custom HTTP client with a User-Agent header
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://4.photonicat.com/ip.php", nil) // Use the IPv4-specific endpoint
	if err != nil {
		fmt.Println("Failed to create request:", err)
		return ""
	}

	// Set the User-Agent header to simulate a curl request
	req.Header.Set("User-Agent", "curl/7.68.0") // Use a typical curl user agent string

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Failed to send request:", err)
		return ""
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Failed to read response body:", err)
		return ""
	}

	// Return the external IPv4 address as a string
	return string(body)
}

func myExternalIPv6() string {
	// Create a custom HTTP client with a User-Agent header
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://6.photonicat.com/ip.php", nil) // Use the IPv4-specific endpoint
	if err != nil {
		fmt.Println("Failed to create request:", err)
		return ""
	}

	// Set the User-Agent header to simulate a curl request
	req.Header.Set("User-Agent", "curl/7.68.0") // Use a typical curl user agent string

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Failed to send request:", err)
		return ""
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Failed to read response body:", err)
		return ""
	}

	// Return the external IPv4 address as a string
	return string(body)
}

// Get the ping time for a URL.
func getMS(url string) float64 {
	start := time.Now()
	resp, err := http.Get(url)
	if err != nil {
		return 9999
	}
	defer resp.Body.Close()
	return float64(time.Since(start).Milliseconds())
}

// Get minimum and maximum ping times over multiple requests.
func getMSTimes(url string, times int) [2]float64 {
	var mss []float64
	for i := 0; i < times; i++ {
		mss = append(mss, getMS(url))
		time.Sleep(time.Duration(int(Config["retryIntervalMS"].(float64))) * time.Millisecond)
		fmt.Printf("%s, site: %s, ping: %dms\n", time.Now().Format("2006-01-02 15:04:05"), url, int(mss[len(mss)-1]))
	}
	return [2]float64{min(mss), max(mss)}
}

func readConfig() {
	// Read the config file
	config, err := os.ReadFile("config.json")
	if err != nil {
		fmt.Println("Failed to read config.json:", err)
		os.Exit(1)
	}

	// Unmarshal the content into the Config variable
	err = json.Unmarshal(config, &Config)
	if err != nil {
		fmt.Println("Failed to unmarshal config.json:", err)
		os.Exit(2)
	}
}


// Get the SSH client to run OpenWRT commands.
func runOpenWrtCmd(cmd string) string {
	password := Config["password"].(string)

	client, err := ssh.Dial("tcp", openwrtIP+":22", &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password(string(password))}, 
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		fmt.Println("Failed to connect to OpenWRT:", err)
		return ""
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		fmt.Println("Failed to create session:", err)
		return ""
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		fmt.Println("Failed to run command:", err)
		fmt.Println(string(cmd))
		return ""
	}
	return string(output)
}

// Get a list of OpenWRT nodes.
func getOpenWrtNodeList() []string {
	preferredNodeKeywords_config := Config["preferredNodeKeywords"].([]interface{})
	dislikedNodeKeywords_config := Config["dislikedNodeKeywords"].([]interface{})
	preferredNodeKeywords := make([]string, len(preferredNodeKeywords_config))
	dislikedNodeKeywords := make([]string, len(dislikedNodeKeywords_config))
	for i, keyword := range preferredNodeKeywords_config {
		preferredNodeKeywords[i] = keyword.(string)
	}
	for i, keyword := range dislikedNodeKeywords_config {
		dislikedNodeKeywords[i] = keyword.(string)
	}

	content := runOpenWrtCmd("uci show passwall")
	lines := strings.Split(content, "\n")

	var nodeList []string
	var nodeListNames []string
	for _, line := range lines {
		elems := strings.Split(line, "=")
		if len(elems) > 1 && strings.HasSuffix(elems[0], ".remarks") {
			locationRemark := elems[1]
			if len(locationRemark) > 2 && containsKeyword(locationRemark, preferredNodeKeywords) && !containsKeyword(locationRemark, dislikedNodeKeywords) {

				nodeStr := strings.Split(elems[0], ".")[1]
				nodeList = append(nodeList, nodeStr)
				nodeListNames = append(nodeListNames, locationRemark)
			}
		}
	}
	if len(nodeListNames) >= 0 {
		for i, nodeName := range nodeListNames {
			fmt.Println(i, nodeName)
		}
	}else{
		fmt.Println("No candidate nodes found.")
	}
	return nodeList
}

// Helper function to check if a string contains any of the keywords.
func containsKeyword(s string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(s, keyword) {
			return true
		}
	}
	return false
}

// Switch to a random OpenWRT node.
func switchToARandomNode() {
	fmt.Println("Switching to a random node...")
	time.Sleep(100 * time.Millisecond)
	nodes = getOpenWrtNodeList()
	rand.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })
	cmd := "uci set passwall.@global[0].tcp_node='" + nodes[0] + "' && " + basicRestartCmd
	fmt.Println(cmd)
	fmt.Println(runOpenWrtCmd(cmd))
}

// Switch node if there's no external connection.
func switchIfNoExternal() bool {
	ms := getMSTimes(Config["checkSite"].(string), int(Config["retryTimes"].(float64)))
	if ms[0] > Config["checkSiteTimeoutMS"].(float64) || ms[1] == 9999 {
		switchToARandomNode()
		return false
	}
	return true
}

// Get the current node.
func getCurrentNode() string {
	cmd := "uci get passwall.@global[0].tcp_node"
	currNode := runOpenWrtCmd(cmd)
	currNode = strings.TrimSpace(currNode)
	fmt.Println(currNode)
	cmd = "uci show passwall." + currNode
	return runOpenWrtCmd(cmd)
}
// GetDefaultGateway reads the /proc/net/route file to find the default gateway.
func getDefaultGateway() (string, error) {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return "", fmt.Errorf("failed to open /proc/net/route: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		// Check if the route is the default route (destination is 00000000)
		if len(fields) < 3 || fields[1] != "00000000" {
			continue
		}

		// The gateway is in the 3rd field (column)
		gatewayHex := fields[2]
		gateway, err := hexToIP(gatewayHex)
		if err != nil {
			return "", err
		}

		return gateway, nil
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read /proc/net/route: %v", err)
	}

	return "", fmt.Errorf("no default gateway found")
}

// Convert a hexadecimal IP address to a string (IPv4).
func hexToIP(hexIP string) (string, error) {
	var ipInt uint32
	_, err := fmt.Sscanf(hexIP, "%x", &ipInt)
	if err != nil {
		return "", fmt.Errorf("invalid IP hex string")
	}

	ip := net.IPv4(byte(ipInt), byte(ipInt>>8), byte(ipInt>>16), byte(ipInt>>24))
	return ip.String(), nil
}

// Min and Max functions.
func min(nums []float64) float64 {
	m := nums[0]
	for _, n := range nums[1:] {
		if n < m {
			m = n
		}
	}
	return m
}

func max(nums []float64) float64 {
	m := nums[0]
	for _, n := range nums[1:] {
		if n > m {
			m = n
		}
	}
	return m
}

func main() {
	readConfig()
	openwrtIP = Config["openwrtIP"].(string)
	if openwrtIP == "" {
		openwrtIP, err := getDefaultGateway()
		if err != nil {
			fmt.Println("Error getting default gateway:", err)
			return
		} else {
			fmt.Println("Trying with default gateway OpenWRT IP =", openwrtIP)
		}
	}

	allNodes := getOpenWrtNodeList()
	fmt.Println("Total available node candidates: ",len(allNodes))
	fmt.Println(allNodes)

	for {
		ipv4 := myExternalIPv4()
		fmt.Println("My IPv4 =", ipv4)
		fmt.Println(getCurrentNode())
		if switchIfNoExternal() {
			fmt.Println(getCurrentNode())
			fmt.Println("Current node is working")
			time.Sleep(360 * time.Second)
		} else {
			fmt.Println("Changed to a new node, retrying...")
		}
	}
}
