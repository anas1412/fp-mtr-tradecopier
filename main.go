package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ────────────────────────────────────────────────────────────────────────────
// Constants
// ────────────────────────────────────────────────────────────────────────────

const (
	baseURL    = "https://mtr-platform.fundingpips.com"
	brokerID   = "1"
	systemUUID = "beedbea9-c757-46ad-b93b-a52ba2c3d648"
	pollMs     = 200
	configFile = "copier-config.json"
)

// ────────────────────────────────────────────────────────────────────────────
// Saved Config
// ────────────────────────────────────────────────────────────────────────────

type SavedConfig struct {
	Email      string  `json:"email"`
	Password   string  `json:"password"`
	MasterID   string  `json:"master_id"`
	SlaveID    string  `json:"slave_id"`
	Multiplier float64 `json:"multiplier"`
}

func loadConfig() *SavedConfig {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil
	}
	var c SavedConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	return &c
}

func saveConfig(c SavedConfig) {
	data, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(configFile, data, 0600)
}

func clearConfig() {
	os.Remove(configFile)
}

// ────────────────────────────────────────────────────────────────────────────
// Styles
// ────────────────────────────────────────────────────────────────────────────

var (
	clrTeal  = lipgloss.Color("#3FD1C2")
	clrRed   = lipgloss.Color("#F0546E")
	clrAmber = lipgloss.Color("#FDA522")
	clrBlue  = lipgloss.Color("#25C2EE")
	clrGray  = lipgloss.Color("#95A4BB")
	clrPanel = lipgloss.Color("#2E3642")
	clrWhite = lipgloss.Color("#BBC4D3")
	clrBg    = lipgloss.Color("#1A2028")

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(clrBlue).
			BorderStyle(lipgloss.DoubleBorder()).
			BorderForeground(clrBlue).
			Padding(0, 3)

	stylePanel = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(clrPanel).
			Padding(0, 1)

	stylePanelMaster = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(clrBlue).
				Padding(0, 1)

	stylePanelSlave = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(clrAmber).
			Padding(0, 1)

	styleSectionHeader = lipgloss.NewStyle().
				Foreground(clrGray).
				BorderStyle(lipgloss.NormalBorder()).
				BorderBottom(true).
				BorderForeground(clrPanel)

	styleLabel = lipgloss.NewStyle().Foreground(clrGray)
	styleValue = lipgloss.NewStyle().Foreground(clrWhite).Bold(true)
	styleGreen = lipgloss.NewStyle().Foreground(clrTeal).Bold(true)
	styleRed   = lipgloss.NewStyle().Foreground(clrRed).Bold(true)
	styleAmber = lipgloss.NewStyle().Foreground(clrAmber)
	styleBlue  = lipgloss.NewStyle().Foreground(clrBlue)
	styleDim   = lipgloss.NewStyle().Foreground(clrGray)
	styleBold  = lipgloss.NewStyle().Foreground(clrWhite).Bold(true)

	styleLogInfo  = lipgloss.NewStyle().Foreground(clrBlue)
	styleLogOK    = lipgloss.NewStyle().Foreground(clrTeal)
	styleLogErr   = lipgloss.NewStyle().Foreground(clrRed)
	styleLogTrade = lipgloss.NewStyle().Foreground(clrAmber).Bold(true)

	styleInput = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(clrGray).
			Padding(0, 1)

	styleFocused = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(clrBlue).
			Padding(0, 1)

	styleBtn = lipgloss.NewStyle().
			Background(clrBlue).
			Foreground(clrBg).
			Bold(true).
			Padding(0, 3)

	styleBtnFocused = lipgloss.NewStyle().
			Background(clrTeal).
			Foreground(clrBg).
			Bold(true).
			Padding(0, 3)

	styleBadgeGreen = lipgloss.NewStyle().
			Background(clrTeal).
			Foreground(clrBg).
			Bold(true).
			Padding(0, 1)

	styleBadgeRed = lipgloss.NewStyle().
			Background(clrRed).
			Foreground(clrWhite).
			Bold(true).
			Padding(0, 1)

	styleBadgeAmber = lipgloss.NewStyle().
			Background(clrAmber).
			Foreground(clrBg).
			Bold(true).
			Padding(0, 1)

	styleAccountRow = lipgloss.NewStyle().
			Padding(0, 2)

	styleAccountRowSelected = lipgloss.NewStyle().
				Background(clrPanel).
				Foreground(clrWhite).
				Bold(true).
				Padding(0, 2)

	styleAccountRowFocused = lipgloss.NewStyle().
				Background(clrBlue).
				Foreground(clrBg).
				Bold(true).
				Padding(0, 2)

	styleTag = lipgloss.NewStyle().
			Background(clrPanel).
			Foreground(clrGray).
			Padding(0, 1)

	styleMasterTag = lipgloss.NewStyle().
			Background(clrBlue).
			Foreground(clrBg).
			Bold(true).
			Padding(0, 1)

	styleSlaveTag = lipgloss.NewStyle().
			Background(clrAmber).
			Foreground(clrBg).
			Bold(true).
			Padding(0, 1)
)

// ────────────────────────────────────────────────────────────────────────────
// API Types
// ────────────────────────────────────────────────────────────────────────────

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	BrokerID string `json:"brokerId"`
}

type LoginResponse struct {
	Email    string       `json:"email"`
	Accounts []APIAccount `json:"accounts"`
}

type APIAccount struct {
	TradingAccountID string `json:"tradingAccountId"`
	TradingApiToken  string `json:"tradingApiToken"`
	UUID             string `json:"uuid"`
	Offer            struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Currency    string `json:"currency"`
		Demo        bool   `json:"demo"`
	} `json:"offer"`
}

type OpenPositionsResponse struct {
	Positions []Position `json:"positions"`
}

type Position struct {
	ID         string `json:"id"`
	Symbol     string `json:"symbol"`
	Volume     string `json:"volume"`
	Side       string `json:"side"`
	OpenPrice  string `json:"openPrice"`
	StopLoss   string `json:"stopLoss"`
	TakeProfit string `json:"takeProfit"`
	Profit     string `json:"profit"`
	Swap       string `json:"swap"`
	OpenTime   string `json:"openTime"`
}

type OpenPositionRequest struct {
	Instrument string  `json:"instrument"`
	OrderSide  string  `json:"orderSide"`
	Volume     float64 `json:"volume"`
	SlPrice    float64 `json:"slPrice"`
	TpPrice    float64 `json:"tpPrice"`
	IsMobile   bool    `json:"isMobile"`
}

type ClosePositionRequest struct {
	PositionID string `json:"positionId"`
	Instrument string `json:"instrument"`
	OrderSide  string `json:"orderSide"`
	Volume     string `json:"volume"`
}

type APIResponse struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage"`
}

type BalanceResponse struct {
	Balance    string `json:"balance"`
	Equity     string `json:"equity"`
	Profit     string `json:"profit"`
	FreeMargin string `json:"freeMargin"`
	Currency   string `json:"currency"`
}

// ────────────────────────────────────────────────────────────────────────────
// HTTP Client
// ────────────────────────────────────────────────────────────────────────────

type Client struct {
	http            *http.Client
	tradingApiToken string
	accountID       string
	accountName     string
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{http: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

// browserHeaders sets headers that match what the MatchTrader web app sends,
// required to pass Cloudflare's bot detection.
func browserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", baseURL+"/sign-in")
	req.Header.Set("Sec-Ch-Ua", `"Google Chrome";v="125", "Chromium";v="125", "Not.A/Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Linux"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
}

// LoginAll logs in and returns ALL accounts (used for selection screen)
func (c *Client) LoginAll(email, password string) ([]APIAccount, error) {
	body, _ := json.Marshal(LoginRequest{Email: email, Password: password, BrokerID: brokerID})
	req, err := http.NewRequest("POST", baseURL+"/manager/co-login", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	browserHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var lr LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(lr.Accounts) == 0 {
		return nil, fmt.Errorf("no accounts found")
	}
	return lr.Accounts, nil
}

// SelectAccount sets the client to use a specific account from the already-logged-in session
func (c *Client) SelectAccount(accounts []APIAccount, accountID string) error {
	for _, a := range accounts {
		if a.TradingAccountID == accountID {
			c.tradingApiToken = a.TradingApiToken
			c.accountID = a.TradingAccountID
			c.accountName = a.Offer.Description
			if c.accountName == "" {
				c.accountName = a.Offer.Name
			}
			return nil
		}
	}
	return fmt.Errorf("account %s not found", accountID)
}

func (c *Client) do(method, path string, body interface{}) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Auth-trading-api", c.tradingApiToken)
	browserHeaders(req)
	return c.http.Do(req)
}

func (c *Client) GetOpenPositions() ([]Position, error) {
	resp, err := c.do("GET", "/mtr-api/"+systemUUID+"/open-positions", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("unauthorized (session expired)")
	}
	var r OpenPositionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return r.Positions, nil
}

func (c *Client) GetBalance() (*BalanceResponse, error) {
	resp, err := c.do("GET", "/mtr-api/"+systemUUID+"/balance", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r BalanceResponse
	json.NewDecoder(resp.Body).Decode(&r)
	return &r, nil
}

func (c *Client) OpenPosition(p Position, multiplier float64) error {
	vol, _ := strconv.ParseFloat(p.Volume, 64)
	sl, _ := strconv.ParseFloat(p.StopLoss, 64)
	tp, _ := strconv.ParseFloat(p.TakeProfit, 64)
	vol = math.Round(vol*multiplier*100) / 100
	resp, err := c.do("POST", "/mtr-api/"+systemUUID+"/position/open", OpenPositionRequest{
		Instrument: p.Symbol, OrderSide: p.Side, Volume: vol,
		SlPrice: sl, TpPrice: tp, IsMobile: false,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var ar APIResponse
	json.NewDecoder(resp.Body).Decode(&ar)
	if ar.Status != "OK" {
		return fmt.Errorf("%s", ar.ErrorMessage)
	}
	return nil
}

func (c *Client) ClosePosition(p Position) error {
	resp, err := c.do("POST", "/mtr-api/"+systemUUID+"/positions/close",
		[]ClosePositionRequest{{p.ID, p.Symbol, p.Side, p.Volume}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var ar APIResponse
	json.NewDecoder(resp.Body).Decode(&ar)
	if ar.Status != "OK" {
		return fmt.Errorf("%s", ar.ErrorMessage)
	}
	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// Log
// ────────────────────────────────────────────────────────────────────────────

type LogKind int

const (
	LogInfo LogKind = iota
	LogOK
	LogErr
	LogTrade
)

type LogEntry struct {
	Time    time.Time
	Kind    LogKind
	Message string
}

func (e LogEntry) Render() string {
	ts := styleDim.Render(e.Time.Format("15:04:05"))
	var icon, msg string
	switch e.Kind {
	case LogOK:
		icon = styleLogOK.Render("✓")
		msg = styleLogOK.Render(e.Message)
	case LogErr:
		icon = styleLogErr.Render("✗")
		msg = styleLogErr.Render(e.Message)
	case LogTrade:
		icon = styleLogTrade.Render("◆")
		msg = styleLogTrade.Render(e.Message)
	default:
		icon = styleLogInfo.Render("·")
		msg = styleLogInfo.Render(e.Message)
	}
	return fmt.Sprintf("%s %s %s", ts, icon, msg)
}

// ────────────────────────────────────────────────────────────────────────────
// Screens
// ────────────────────────────────────────────────────────────────────────────

type screen int

const (
	screenLogin    screen = iota // email + password
	screenAccounts               // pick master then slave
	screenMultiplier             // enter lot multiplier
	screenCopying               // live copier
)

type pickStep int

const (
	pickMaster pickStep = iota
	pickSlave
)

// ────────────────────────────────────────────────────────────────────────────
// Model
// ────────────────────────────────────────────────────────────────────────────

type statsData struct {
	mu            sync.Mutex
	masterPos     []Position
	slavePos      []Position
	masterBalance *BalanceResponse
	slaveBalance  *BalanceResponse
	lastPoll      time.Time
	copied        int
	closed        int
	errors        int
}

type Model struct {
	screen  screen
	width   int
	height  int
	spinner spinner.Model

	// Login screen
	emailInput textinput.Model
	passInput  textinput.Model
	loginFocus int // 0=email 1=pass 2=button
	loginErr   string
	connecting bool

	// Account selection
	accounts   []APIAccount
	cursor     int
	masterID   string
	slaveID    string
	pickStep   pickStep
	sharedJar  *Client // holds the co-auth cookie for both logins

	// Multiplier screen
	multInput  textinput.Model
	multErr    string

	// Copier
	master     *Client
	slave      *Client
	multiplier float64
	stats      statsData
	logs       []LogEntry
	known      map[string]Position
}

func newModel() Model {
	sp := spinner.New()
	sp.Spinner = spinner.Points
	sp.Style = lipgloss.NewStyle().Foreground(clrBlue)

	email := textinput.New()
	email.Placeholder = "you@email.com"
	email.Focus()
	email.Width = 38

	pass := textinput.New()
	pass.Placeholder = "password"
	pass.EchoMode = textinput.EchoPassword
	pass.EchoCharacter = '•'
	pass.Width = 38

	mult := textinput.New()
	mult.Placeholder = "1.0"
	mult.SetValue("1.0")
	mult.Width = 20
	mult.Focus()

	m := Model{
		screen:     screenLogin,
		spinner:    sp,
		emailInput: email,
		passInput:  pass,
		multInput:  mult,
		known:      make(map[string]Position),
	}

	// Pre-fill from saved config if available
	if cfg := loadConfig(); cfg != nil {
		m.emailInput.SetValue(cfg.Email)
		m.passInput.SetValue(cfg.Password)
		m.multInput.SetValue(fmt.Sprintf("%.2f", cfg.Multiplier))
	}

	return m
}

// ────────────────────────────────────────────────────────────────────────────
// Messages
// ────────────────────────────────────────────────────────────────────────────

type msgLoginDone  struct{ accounts []APIAccount; err error }
type msgSeedDone   struct{ count int; err error }
type msgPollTick   struct{}

// ────────────────────────────────────────────────────────────────────────────
// Init
// ────────────────────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, textinput.Blink)
}

// ────────────────────────────────────────────────────────────────────────────
// Update
// ────────────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Spinner always ticks
	var spCmd tea.Cmd
	m.spinner, spCmd = m.spinner.Update(msg)
	cmds = append(cmds, spCmd)

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch m.screen {
		case screenLogin:
			return m.handleLoginKey(msg)
		case screenAccounts:
			return m.handleAccountsKey(msg)
		case screenMultiplier:
			return m.handleMultiplierKey(msg)
		case screenCopying:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "l":
				return m.logout()
			}
		}

	case msgLoginDone:
		m.connecting = false
		if msg.err != nil {
			m.loginErr = msg.err.Error()
		} else {
			m.accounts = msg.accounts
			m.screen = screenAccounts
			m.cursor = 0
		}

	case msgSeedDone:
		m.screen = screenCopying
		m.log(LogOK, fmt.Sprintf("Authenticated — master #%s → slave #%s  ×%.2f",
			m.master.accountID, m.slave.accountID, m.multiplier))
		if msg.err != nil {
			m.log(LogErr, "Seed error: "+msg.err.Error())
		} else {
			m.log(LogInfo, fmt.Sprintf("Ready — %d existing position(s) seeded (not copied)", msg.count))
		}
		cmds = append(cmds, m.cmdSchedulePoll())

	case msgPollTick:
		m.doPoll()
		cmds = append(cmds, m.cmdSchedulePoll())
	}

	return m, tea.Batch(cmds...)
}

// ────────────────────────────────────────────────────────────────────────────
// Key handlers
// ────────────────────────────────────────────────────────────────────────────

func (m Model) handleLoginKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "tab", "down":
		m.loginFocus = (m.loginFocus + 1) % 3
		m.emailInput.Blur()
		m.passInput.Blur()
		if m.loginFocus == 0 {
			m.emailInput.Focus()
		} else if m.loginFocus == 1 {
			m.passInput.Focus()
		}
		cmds = append(cmds, textinput.Blink)

	case "shift+tab", "up":
		m.loginFocus = (m.loginFocus + 2) % 3
		m.emailInput.Blur()
		m.passInput.Blur()
		if m.loginFocus == 0 {
			m.emailInput.Focus()
		} else if m.loginFocus == 1 {
			m.passInput.Focus()
		}
		cmds = append(cmds, textinput.Blink)

	case "enter":
		if m.loginFocus == 2 || m.loginFocus == 1 {
			return m.doLogin()
		}
		m.loginFocus++
		m.emailInput.Blur()
		m.passInput.Focus()
		cmds = append(cmds, textinput.Blink)
	}

	var cmd tea.Cmd
	if m.loginFocus == 0 {
		m.emailInput, cmd = m.emailInput.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.loginFocus == 1 {
		m.passInput, cmd = m.passInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleAccountsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.accounts)-1 {
			m.cursor++
		}
	case "enter", " ":
		selected := m.accounts[m.cursor].TradingAccountID
		if m.pickStep == pickMaster {
			m.masterID = selected
			m.pickStep = pickSlave
			// Reset cursor, exclude master from slave selection
			m.cursor = 0
			if m.accounts[m.cursor].TradingAccountID == m.masterID && len(m.accounts) > 1 {
				m.cursor = 1
			}
		} else {
			// Don't allow same account for both
			if selected == m.masterID {
				return m, nil
			}
			m.slaveID = selected
			m.screen = screenMultiplier
			m.multInput.Focus()
		}
	case "esc", "b":
		if m.pickStep == pickSlave {
			m.pickStep = pickMaster
			m.masterID = ""
		}
	}
	return m, textinput.Blink
}

func (m Model) handleMultiplierKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "b":
		m.screen = screenAccounts
		m.pickStep = pickSlave
		m.slaveID = ""
		return m, nil
	case "enter":
		mult, err := strconv.ParseFloat(strings.TrimSpace(m.multInput.Value()), 64)
		if err != nil || mult <= 0 {
			m.multErr = "Must be a positive number (e.g. 1.0)"
			return m, nil
		}
		m.multErr = ""
		m.multiplier = mult
		return m.startCopier()
	}
	var cmd tea.Cmd
	m.multInput, cmd = m.multInput.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// ────────────────────────────────────────────────────────────────────────────
// Actions
// ────────────────────────────────────────────────────────────────────────────

func (m Model) doLogin() (tea.Model, tea.Cmd) {
	email := strings.TrimSpace(m.emailInput.Value())
	pass := m.passInput.Value()
	if email == "" || pass == "" {
		m.loginErr = "Email and password required"
		return m, nil
	}
	m.loginErr = ""
	m.connecting = true
	// Create one shared client that holds the co-auth cookie
	m.sharedJar = NewClient()

	return m, func() tea.Msg {
		accounts, err := m.sharedJar.LoginAll(email, pass)
		return msgLoginDone{accounts, err}
	}
}

func (m Model) startCopier() (tea.Model, tea.Cmd) {
	email := strings.TrimSpace(m.emailInput.Value())

	// Save config
	saveConfig(SavedConfig{
		Email:      email,
		Password:   m.passInput.Value(),
		MasterID:   m.masterID,
		SlaveID:    m.slaveID,
		Multiplier: m.multiplier,
	})

	// Both clients share the same jar (same login session = same co-auth cookie)
	// but each needs its own tradingApiToken for the selected account
	m.master = &Client{http: m.sharedJar.http}
	m.slave = NewClient()

	// Slave needs its own session (different tradingApiToken)
	pass := m.passInput.Value()

	return m, func() tea.Msg {
		// Select master from already-logged-in accounts
		if err := m.master.SelectAccount(m.accounts, m.masterID); err != nil {
			return msgLoginDone{err: fmt.Errorf("master: %w", err)}
		}

		// Log slave in separately to get its own tradingApiToken
		slaveAccounts, err := m.slave.LoginAll(email, pass)
		if err != nil {
			return msgLoginDone{err: fmt.Errorf("slave login: %w", err)}
		}
		if err := m.slave.SelectAccount(slaveAccounts, m.slaveID); err != nil {
			return msgLoginDone{err: fmt.Errorf("slave: %w", err)}
		}

		// Seed existing master positions
		pos, err := m.master.GetOpenPositions()
		if err != nil {
			return msgSeedDone{err: err}
		}
		for _, p := range pos {
			m.known[p.ID] = p
		}
		return msgSeedDone{count: len(pos)}
	}
}

func (m Model) logout() (tea.Model, tea.Cmd) {
	clearConfig()
	fresh := newModel()
	fresh.width = m.width
	fresh.height = m.height
	// Re-init the fresh model (spinner tick, blink) so the login screen works immediately
	return fresh, fresh.Init()
}

func (m *Model) log(kind LogKind, msg string) {
	m.logs = append(m.logs, LogEntry{time.Now(), kind, msg})
	if len(m.logs) > 500 {
		m.logs = m.logs[len(m.logs)-500:]
	}
}

func (m Model) cmdSeed() tea.Cmd {
	return func() tea.Msg {
		pos, err := m.master.GetOpenPositions()
		if err != nil {
			return msgSeedDone{err: err}
		}
		for _, p := range pos {
			m.known[p.ID] = p
		}
		return msgSeedDone{count: len(pos)}
	}
}

func (m Model) cmdSchedulePoll() tea.Cmd {
	return tea.Tick(pollMs*time.Millisecond, func(time.Time) tea.Msg {
		return msgPollTick{}
	})
}

// ────────────────────────────────────────────────────────────────────────────
// Poll
// ────────────────────────────────────────────────────────────────────────────

func posMap(positions []Position) map[string]Position {
	m := make(map[string]Position, len(positions))
	for _, p := range positions {
		m[p.ID] = p
	}
	return m
}

func (m *Model) doPoll() {
	if m.screen != screenCopying {
		return
	}
	masterPos, err := m.master.GetOpenPositions()
	if err != nil {
		m.stats.errors++
		m.log(LogErr, "Poll: "+err.Error())
		return
	}
	current := posMap(masterPos)

	// New → open on slave
	for id, pos := range current {
		if _, known := m.known[id]; !known {
			side := styleGreen.Render(pos.Side)
			if pos.Side == "SELL" {
				side = styleRed.Render(pos.Side)
			}
			m.log(LogTrade, fmt.Sprintf("NEW %s  %-8s  vol=%-6s  @ %s",
				side, pos.Symbol, pos.Volume, pos.OpenPrice))
			if err := m.slave.OpenPosition(pos, m.multiplier); err != nil {
				m.stats.errors++
				m.log(LogErr, "  ↳ failed to open on slave: "+err.Error())
			} else {
				m.stats.copied++
				scaledVol := pos.Volume
				if v, e2 := strconv.ParseFloat(pos.Volume, 64); e2 == nil {
					scaledVol = fmt.Sprintf("%.2f", math.Round(v*m.multiplier*100)/100)
				}
				m.log(LogOK, fmt.Sprintf("  ↳ opened on slave  vol=%s ✓", scaledVol))
			}
		}
	}

	// Closed → close on slave
	for id, pos := range m.known {
		if _, stillOpen := current[id]; !stillOpen {
			m.log(LogTrade, fmt.Sprintf("CLOSED %-6s %-8s → closing slave...", pos.Side, pos.Symbol))
			slavePos, err := m.slave.GetOpenPositions()
			if err != nil {
				m.stats.errors++
				m.log(LogErr, "  ↳ could not fetch slave positions: "+err.Error())
				continue
			}
			found := false
			for _, sp := range slavePos {
				if sp.Symbol == pos.Symbol && sp.Side == pos.Side {
					if err := m.slave.ClosePosition(sp); err != nil {
						m.stats.errors++
						m.log(LogErr, "  ↳ failed to close: "+err.Error())
					} else {
						m.stats.closed++
						m.log(LogOK, fmt.Sprintf("  ↳ closed on slave ✓  P&L was %s", sp.Profit))
						found = true
					}
					break
				}
			}
			if !found {
				m.log(LogErr, fmt.Sprintf("  ↳ no matching slave position for %s %s", pos.Side, pos.Symbol))
			}
		}
	}

	m.known = current

	masterBal, _ := m.master.GetBalance()
	slaveBal, _ := m.slave.GetBalance()
	slavePos, _ := m.slave.GetOpenPositions()

	m.stats.mu.Lock()
	m.stats.masterPos = masterPos
	m.stats.slavePos = slavePos
	m.stats.masterBalance = masterBal
	m.stats.slaveBalance = slaveBal
	m.stats.lastPoll = time.Now()
	m.stats.mu.Unlock()
}

// ────────────────────────────────────────────────────────────────────────────
// Views
// ────────────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	switch m.screen {
	case screenLogin:
		return m.viewLogin()
	case screenAccounts:
		return m.viewAccounts()
	case screenMultiplier:
		return m.viewMultiplier()
	case screenCopying:
		return m.viewCopying()
	}
	return ""
}

func (m Model) viewLogin() string {
	title := styleTitle.Render("  ◈  FundingPips Trade Copier  ")

	// Pre-fill notice
	notice := ""
	if cfg := loadConfig(); cfg != nil {
		notice = styleDim.Render(fmt.Sprintf("  Saved config found — last used: master #%s → slave #%s  ×%.2f",
			cfg.MasterID, cfg.SlaveID, cfg.Multiplier))
	}

	emailBox := styleInput.Render(m.emailInput.View())
	if m.loginFocus == 0 {
		emailBox = styleFocused.Render(m.emailInput.View())
	}
	passBox := styleInput.Render(m.passInput.View())
	if m.loginFocus == 1 {
		passBox = styleFocused.Render(m.passInput.View())
	}

	btn := styleBtn.Render("   Sign In   ")
	if m.loginFocus == 2 {
		btn = styleBtnFocused.Render(" ▶  Sign In   ")
	}

	errLine := ""
	if m.loginErr != "" {
		errLine = "\n" + styleLogErr.Render("  ⚠  "+m.loginErr)
	}
	status := ""
	if m.connecting {
		status = "\n" + m.spinner.View() + styleBlue.Render("  Signing in...")
	}

	help := styleDim.Render("  Tab · ↑↓  navigate    Enter  confirm    Ctrl+C  quit")

	inner := lipgloss.JoinVertical(lipgloss.Left,
		title, "",
		notice, "",
		styleLabel.Render("  Email"), emailBox,
		"",
		styleLabel.Render("  Password"), passBox,
		"",
		btn,
		errLine, status,
		"", help,
	)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
}

func (m Model) viewAccounts() string {
	title := styleTitle.Render("  ◈  Select Accounts  ")

	var stepLabel string
	if m.pickStep == pickMaster {
		stepLabel = styleMasterTag.Render(" MASTER ") + styleDim.Render("  Choose the account to copy FROM")
	} else {
		stepLabel = styleSlaveTag.Render(" SLAVE ") + styleDim.Render("  Choose the account to copy TO") +
			"  " + styleDim.Render("master: ") + styleBlue.Render("#"+m.masterID)
	}

	var rows []string
	for i, acc := range m.accounts {
		isMaster := acc.TradingAccountID == m.masterID
		isSlave := acc.TradingAccountID == m.slaveID

		name := acc.Offer.Description
		if name == "" {
			name = acc.Offer.Name
		}
		acctType := "DEMO"
		if !acc.Offer.Demo {
			acctType = "LIVE"
		}

		tags := ""
		if isMaster {
			tags = "  " + styleMasterTag.Render(" MASTER ")
		}
		if isSlave {
			tags = "  " + styleSlaveTag.Render(" SLAVE ")
		}

		// Dim accounts that are already selected as master during slave pick
		isDisabled := m.pickStep == pickSlave && isMaster

		line := fmt.Sprintf("  #%-10s  %-8s  %-30s  [%s]%s",
			acc.TradingAccountID,
			acc.Offer.Currency,
			name,
			acctType,
			tags,
		)

		var row string
		if isDisabled {
			row = styleDim.Render(line + "  (already master)")
		} else if i == m.cursor {
			row = styleAccountRowFocused.Render(line)
		} else if isMaster || isSlave {
			row = styleAccountRowSelected.Render(line)
		} else {
			row = styleAccountRow.Render(line)
		}
		rows = append(rows, row)
	}

	list := stylePanel.Render(strings.Join(rows, "\n"))

	help := styleDim.Render("  ↑↓ / j k  navigate    Enter / Space  select    Esc / b  back    q  quit")

	inner := lipgloss.JoinVertical(lipgloss.Left,
		title, "",
		stepLabel, "",
		list,
		"", help,
	)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
}

func (m Model) viewMultiplier() string {
	title := styleTitle.Render("  ◈  Lot Multiplier  ")

	summary := lipgloss.JoinHorizontal(lipgloss.Top,
		styleDim.Render("Master  "), styleMasterTag.Render(" #"+m.masterID+" "),
		styleDim.Render("  →  "),
		styleSlaveTag.Render(" #"+m.slaveID+" "), styleDim.Render("  Slave"),
	)

	desc := styleDim.Render("  Scale slave lot size relative to master.\n  1.0 = same size  ·  0.5 = half  ·  2.0 = double")

	box := styleFocused.Render(m.multInput.View())

	errLine := ""
	if m.multErr != "" {
		errLine = "\n" + styleLogErr.Render("  ⚠  "+m.multErr)
	}

	btn := styleBtnFocused.Render(" ▶  Start Copier  ")
	help := styleDim.Render("  Enter  confirm    Esc / b  back    Ctrl+C  quit")

	inner := lipgloss.JoinVertical(lipgloss.Left,
		title, "",
		summary, "",
		desc, "",
		box,
		errLine, "",
		btn,
		"", help,
	)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
}

func (m Model) viewCopying() string {
	m.stats.mu.Lock()
	masterPos := m.stats.masterPos
	slavePos := m.stats.slavePos
	masterBal := m.stats.masterBalance
	slaveBal := m.stats.slaveBalance
	copied := m.stats.copied
	closed := m.stats.closed
	errors := m.stats.errors
	lastPoll := m.stats.lastPoll
	m.stats.mu.Unlock()

	// Header
	pollAge := ""
	if !lastPoll.IsZero() {
		ms := time.Since(lastPoll).Milliseconds()
		pollAge = styleDim.Render(fmt.Sprintf("  ⟳ %dms", ms))
	}
	header := lipgloss.JoinHorizontal(lipgloss.Center,
		styleTitle.Render("  ◈  FundingPips Trade Copier  "),
		"  ", styleBadgeGreen.Render(" LIVE "), pollAge,
	)

	// Stats
	cpBadge := styleBadgeGreen.Render(fmt.Sprintf(" ✓ %d copied ", copied))
	clBadge := styleBadgeAmber.Render(fmt.Sprintf(" ⊘ %d closed ", closed))
	errBadge := styleDim.Render("")
	if errors > 0 {
		errBadge = styleBadgeRed.Render(fmt.Sprintf(" ✗ %d errors ", errors))
	}
	mult := styleDim.Render(fmt.Sprintf("  ×%.2f", m.multiplier))
	statsBar := lipgloss.JoinHorizontal(lipgloss.Top, cpBadge, "  ", clBadge, "  ", errBadge, mult)

	// Account panels
	panelW := 56
	masterPanel := m.renderAccount("MASTER", m.master, masterBal, masterPos, stylePanelMaster, panelW)
	slavePanel := m.renderAccount("SLAVE", m.slave, slaveBal, slavePos, stylePanelSlave, panelW)
	panels := lipgloss.JoinHorizontal(lipgloss.Top, masterPanel, "  ", slavePanel)

	// Log
	logView := m.renderLog()

	footer := styleDim.Render("  q  quit    l  logout")

	return lipgloss.JoinVertical(lipgloss.Left,
		header, "",
		statsBar, "",
		panels, "",
		logView, "",
		footer,
	)
}

func (m Model) renderAccount(label string, c *Client, bal *BalanceResponse, positions []Position, st lipgloss.Style, w int) string {
	var acctLabel string
	if label == "MASTER" {
		acctLabel = styleBlue.Render("⬟ MASTER") + "  " + styleLabel.Render("#"+c.accountID)
	} else {
		acctLabel = styleAmber.Render("⬟ SLAVE") + "   " + styleLabel.Render("#"+c.accountID)
	}

	var lines []string
	lines = append(lines, acctLabel, styleDim.Render(c.accountName), "")

	if bal != nil {
		pnl := bal.Profit
		pnlStyled := styleGreen.Render(pnl)
		if v, err := strconv.ParseFloat(pnl, 64); err == nil && v < 0 {
			pnlStyled = styleRed.Render(pnl)
		}
		lines = append(lines,
			styleLabel.Render("Balance  ")+styleBold.Render(bal.Balance+" "+bal.Currency),
			styleLabel.Render("Equity   ")+styleBold.Render(bal.Equity)+"   "+styleLabel.Render("P&L ")+pnlStyled,
			styleLabel.Render("Margin   ")+styleDim.Render(bal.FreeMargin+" free"),
			"",
		)
	}

	if len(positions) == 0 {
		lines = append(lines, styleDim.Render("  — no open positions —"))
	} else {
		lines = append(lines, styleDim.Render(fmt.Sprintf("  %-8s %-5s %-7s %-10s %-9s", "Symbol", "Side", "Vol", "Open@", "P&L")))
		for _, p := range positions {
			sideStr := styleGreen.Render(fmt.Sprintf("%-5s", p.Side))
			if p.Side == "SELL" {
				sideStr = styleRed.Render(fmt.Sprintf("%-5s", p.Side))
			}
			pnl := p.Profit
			pnlStr := styleGreen.Render(fmt.Sprintf("%-9s", pnl))
			if v, err := strconv.ParseFloat(pnl, 64); err == nil && v < 0 {
				pnlStr = styleRed.Render(fmt.Sprintf("%-9s", pnl))
			}
			lines = append(lines, fmt.Sprintf("  %-8s %s %-7s %-10s %s",
				styleValue.Render(fmt.Sprintf("%-8s", p.Symbol)),
				sideStr, p.Volume,
				styleDim.Render(fmt.Sprintf("%-10s", p.OpenPrice)),
				pnlStr,
			))
		}
	}

	return st.Width(w).Render(strings.Join(lines, "\n"))
}

func (m Model) renderLog() string {
	maxLines := 12
	if m.height > 45 {
		maxLines = 18
	}
	var lines []string
	lines = append(lines, styleSectionHeader.Width(m.width-4).Render("  Activity Log"))
	start := 0
	if len(m.logs) > maxLines {
		start = len(m.logs) - maxLines
	}
	for _, e := range m.logs[start:] {
		lines = append(lines, "  "+e.Render())
	}
	if len(m.logs) == 0 {
		lines = append(lines, styleDim.Render("  waiting for activity..."))
	}
	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────
// Update override for msgSeedDone → transition to screenCopying
// ────────────────────────────────────────────────────────────────────────────

// Patch Update to handle screen transition on seed done
func init() {} // nothing needed, handled in Update above

// ────────────────────────────────────────────────────────────────────────────
// Main
// ────────────────────────────────────────────────────────────────────────────

func main() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
