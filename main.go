package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type DHCPLease struct {
	Address    string
	MacAddress string
	Hostname   string
	Vendor     string
}

type MacVendor struct {
	VendorDetails struct {
		Company string `json:"company"`
	} `json:"vendorDetails"`
}

type Credentials struct {
	IP       string `json:"ip"`
	Username string `json:"username"`
}

type RouterConnection struct {
	client  *ssh.Client
	config  *ssh.ClientConfig
	address string
}

type VendorCache struct {
	Vendors map[string]CacheEntry `json:"vendors"`
}

type CacheEntry struct {
	Vendor    string    `json:"vendor"`
	Timestamp time.Time `json:"timestamp"`
}

const (
	initialBackoff = 2 * time.Second
	maxBackoff     = 60 * time.Second
)

func readInput(prompt string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(prompt)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func readPassword(prompt string) string {
	fmt.Print(prompt)
	password, _ := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // Add a newline after password input
	return string(password)
}

func loadCredentials() (Credentials, error) {
	var creds Credentials
	data, err := os.ReadFile("credentials.json")
	if err != nil {
		return creds, err
	}
	err = json.Unmarshal(data, &creds)
	return creds, err
}

func saveCredentials(creds Credentials) error {
	data, err := json.MarshalIndent(creds, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile("credentials.json", data, 0600)
}

func main() {
	var router *RouterConnection
	var err error

	// Initial connection
	router, err = connectToRouter()
	if err != nil {
		fmt.Printf("Error connecting to router: %v\n", err)
		return
	}
	defer router.client.Close()

	for {
		fmt.Println("\nMikroTik Router Utilities")
		fmt.Println("------------------------")
		fmt.Println("1. DHCP Lease Viewer")
		fmt.Println("2. Exit")
		fmt.Print("\nSelect an option: ")

		var choice string
		if _, err := fmt.Scanln(&choice); err != nil {
			fmt.Println("Error reading input. Please try again.")
			continue
		}

		switch choice {
		case "1":
			viewDHCPLeases(router)
		case "2":
			fmt.Println("Goodbye!")
			return
		default:
			fmt.Println("Invalid option. Please try again.")
		}
	}
}

func connectToRouter() (*RouterConnection, error) {
	// Try to load saved credentials
	savedCreds, _ := loadCredentials()

	// Get router IP
	var routerIP string
	if savedCreds.IP != "" {
		routerIP = readInput(fmt.Sprintf("Router IP [%s]: ", savedCreds.IP))
		if routerIP == "" {
			routerIP = savedCreds.IP
		}
	} else {
		routerIP = readInput("Router IP: ")
	}

	// Get username
	var username string
	if savedCreds.Username != "" {
		username = readInput(fmt.Sprintf("Username [%s]: ", savedCreds.Username))
		if username == "" {
			username = savedCreds.Username
		}
	} else {
		username = readInput("Username: ")
	}

	// Get password (never saved)
	password := readPassword("Password: ")

	// Save credentials
	newCreds := Credentials{
		IP:       routerIP,
		Username: username,
	}
	if err := saveCredentials(newCreds); err != nil {
		fmt.Printf("Error saving credentials: %v\n", err)
	}

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", routerIP), config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}

	return &RouterConnection{
		client:  client,
		config:  config,
		address: routerIP,
	}, nil
}

func viewDHCPLeases(router *RouterConnection) {
	session, err := router.client.NewSession()
	if err != nil {
		fmt.Printf("Error creating session: %v\n", err)
		return
	}
	defer session.Close()

	// Execute command to get leases with terse output
	output, err := session.CombinedOutput("/ip dhcp-server lease print terse")
	if err != nil {
		fmt.Printf("Error executing command: %v\n", err)
		return
	}

	// Process output
	leases := parseLeases(string(output))

	// Get vendor information for each lease
	for i := range leases {
		leases[i].Vendor = getMacVendor(leases[i].MacAddress)
	}

	// Display table
	printTable(leases)
}

func parseLeases(output string) []DHCPLease {
	var leases []DHCPLease
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		lease := DHCPLease{}
		parts := strings.Split(line, " ")

		for _, part := range parts {
			switch {
			case strings.HasPrefix(part, "address="):
				lease.Address = strings.TrimPrefix(part, "address=")
			case strings.HasPrefix(part, "mac-address="):
				lease.MacAddress = strings.TrimPrefix(part, "mac-address=")
			case strings.HasPrefix(part, "host-name="):
				lease.Hostname = strings.TrimPrefix(part, "host-name=")
			}
		}

		if lease.Address != "" && lease.MacAddress != "" {
			leases = append(leases, lease)
		}
	}
	return leases
}

func loadVendorCache() VendorCache {
	var cache VendorCache
	data, err := os.ReadFile("vendor_cache.json")
	if err != nil {
		return VendorCache{Vendors: make(map[string]CacheEntry)}
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		return VendorCache{Vendors: make(map[string]CacheEntry)}
	}
	return cache
}

func saveVendorCache(cache VendorCache) error {
	data, err := json.MarshalIndent(cache, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile("vendor_cache.json", data, 0600)
}

func getMacVendor(mac string) string {
	// Get first 3 octets for vendor lookup
	oui := strings.ToUpper(strings.ReplaceAll(mac, ":", "")[:6])

	cache := loadVendorCache()

	// Check cache first
	if entry, exists := cache.Vendors[oui]; exists {
		// Cache entry valid for 30 days
		if time.Since(entry.Timestamp) < 30*24*time.Hour {
			return entry.Vendor
		}
	}

	// If not in cache or expired, query API
	vendor := queryMacVendorAPI(oui)

	// Only cache if we got a valid vendor response
	if vendor != "Unknown" {
		cache.Vendors[oui] = CacheEntry{
			Vendor:    vendor,
			Timestamp: time.Now(),
		}
		if err := saveVendorCache(cache); err != nil {
			fmt.Printf("Warning: Failed to save vendor cache: %v\n", err)
		}
	}

	return vendor
}

func queryMacVendorAPI(oui string) string {
	backoff := initialBackoff
	maxRetries := 3

	for retry := 0; retry < maxRetries; retry++ {
		url := fmt.Sprintf("https://api.macvendors.com/%s", oui)
		client := &http.Client{Timeout: 5 * time.Second}

		resp, err := client.Get(url)
		if err != nil {
			return "Unknown"
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			if retry < maxRetries-1 { // Don't sleep on last retry
				fmt.Printf("Rate limit reached, waiting %v before retry...\n", backoff)
				time.Sleep(backoff)
				backoff *= 2 // Exponential backoff
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}
			return "Rate Limited"
		}

		if resp.StatusCode != http.StatusOK {
			return "Unknown"
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "Unknown"
		}

		var vendor MacVendor
		if err := json.Unmarshal(body, &vendor); err != nil {
			return string(body) // Return plain text if not JSON
		}

		return vendor.VendorDetails.Company
	}

	return "Rate Limited"
}

func printTable(leases []DHCPLease) {
	// Define table style
	columns := []table.Column{
		{Title: "IP", Width: 15},
		{Title: "MAC", Width: 17},
		{Title: "Hostname", Width: 20},
		{Title: "Vendor", Width: 30},
	}

	// Convert leases to rows
	var rows []table.Row
	for _, lease := range leases {
		rows = append(rows, table.Row{
			lease.Address,
			lease.MacAddress,
			lease.Hostname,
			lease.Vendor,
		})
	}

	// Create and style the table
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(len(rows)),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	// Initialize model with default sorting
	m := Model{
		table:         t,
		sortColumn:    0,
		sortAscending: true,
	}
	m.sortTable() // Initial sort

	// Initialize bubbletea program
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
		return
	}
}

// Model represents the UI state
type Model struct {
	table         table.Model
	sortColumn    int
	sortAscending bool
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "right":
			m.sortColumn = (m.sortColumn + 1) % 4
			m.sortTable()
		case "left":
			m.sortColumn = (m.sortColumn - 1 + 4) % 4
			m.sortTable()
		case " ":
			m.sortAscending = !m.sortAscending
			m.sortTable()
		}
	}
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *Model) sortTable() {
	rows := m.table.Rows()
	sort.Slice(rows, func(i, j int) bool {
		a := rows[i][m.sortColumn]
		b := rows[j][m.sortColumn]
		if m.sortAscending {
			return a < b
		}
		return a > b
	})
	m.table.SetRows(rows)
}

// View implements tea.Model
func (m Model) View() string {
	headers := []string{"IP", "MAC", "Hostname", "Vendor"}
	sortIndicator := "↑"
	if !m.sortAscending {
		sortIndicator = "↓"
	}

	// Add sort indicator to current column header
	header := fmt.Sprintf("\nSorting by %s %s (← → to change column, space to toggle order)\n\n",
		headers[m.sortColumn], sortIndicator)

	return header + m.table.View()
}
