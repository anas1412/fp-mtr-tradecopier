package main

import (
	"bytes"
	"crypto/rand"
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

// newUUIDv4 generates a random UUID v4 string (no external deps).
func newUUIDv4() string {
	b := make([]byte, 16)
	rand.Read(b)
	// Set version 4
	b[6] = (b[6] & 0x0f) | 0x40
	// Set variant bits
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ────────────────────────────────────────────────────────────────────────────
// Constants
// ────────────────────────────────────────────────────────────────────────────

const (
	baseURL       = "https://mtr-platform.fundingpips.com"
	brokerID      = "1"
	systemUUID    = "beedbea9-c757-46ad-b93b-a52ba2c3d648"
	defaultPollMs = 500 // safe default; user can tune in settings
	configFile    = "copier-config.json"
)

// ────────────────────────────────────────────────────────────────────────────
// Saved Config
// ────────────────────────────────────────────────────────────────────────────

type SlaveConfig struct {
	AccountID  string  `json:"account_id"`
	Multiplier float64 `json:"multiplier"`
}

type SavedConfig struct {
	Email     string        `json:"email"`
	Password  string        `json:"password"`
	MasterID  string        `json:"master_id"`
	Slaves    []SlaveConfig `json:"slaves"`
	BrowserID string        `json:"browser_id"`
	PollMs    int           `json:"poll_ms"`
}

// savedConfigMigrate embeds SavedConfig and adds the old single-slave fields
// so we can transparently migrate existing configs.
type savedConfigMigrate struct {
	SavedConfig
	SlaveID    string  `json:"slave_id"`
	Multiplier float64 `json:"multiplier"`
}

func loadConfig() *SavedConfig {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil
	}
	var m savedConfigMigrate
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	// Migrate old single-slave format → new multi-slave format
	if len(m.Slaves) == 0 && m.SlaveID != "" {
		m.Slaves = []SlaveConfig{{AccountID: m.SlaveID, Multiplier: m.Multiplier}}
	}
	return &m.SavedConfig
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

	styleFooterBtn = lipgloss.NewStyle().
			Background(clrPanel).
			Foreground(clrWhite).
			Padding(0, 2)

	styleFooterBtnAccent = lipgloss.NewStyle().
				Background(clrBlue).
				Foreground(clrBg).
				Bold(true).
				Padding(0, 2)
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

// ────────────────────────────────────────────────────────────────────────────
// Pending Order Types
// ────────────────────────────────────────────────────────────────────────────

type ActiveOrdersResponse struct {
	Orders []PendingOrder `json:"orders"`
}

type PendingOrder struct {
	ID              string `json:"id"`
	Symbol          string `json:"symbol"`
	Type            string `json:"type"`
	Side            string `json:"side"`
	Volume          string `json:"volume"`
	ActivationPrice string `json:"activationPrice"`
	CreationTime    string `json:"creationTime"`
	StopLoss        string `json:"stopLoss"`
	TakeProfit      string `json:"takeProfit"`
}

type CreatePendingOrderRequest struct {
	Instrument string  `json:"instrument"`
	OrderSide  string  `json:"orderSide"`
	Volume     float64 `json:"volume"`
	Type       string  `json:"type"`
	Price      float64 `json:"price"`
	SlPrice    float64 `json:"slPrice"`
	TpPrice    float64 `json:"tpPrice"`
	IsMobile   bool    `json:"isMobile"`
}

type CancelPendingOrderRequest struct {
	ID         string `json:"id"`
	Instrument string `json:"instrument"`
	OrderSide  string `json:"orderSide"`
	Type       string `json:"type"`
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
	browserID       string
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{http: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

// browserHeaders sets headers that match what the MatchTrader web app sends,
// required to pass Cloudflare's bot detection.
func browserHeaders(req *http.Request, browserID string) {
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
	// Stable browser fingerprint — same ID every time = looks like one real browser
	if browserID != "" {
		req.Header.Set("X-Browser-Id", browserID)
	}
}

// LoginAll logs in and returns ALL accounts (used for selection screen)
func (c *Client) LoginAll(email, password string) ([]APIAccount, error) {
	body, _ := json.Marshal(LoginRequest{Email: email, Password: password, BrokerID: brokerID})
	req, err := http.NewRequest("POST", baseURL+"/manager/co-login", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	browserHeaders(req, c.browserID)

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
	browserHeaders(req, c.browserID)
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

func (c *Client) GetActiveOrders() ([]PendingOrder, error) {
	resp, err := c.do("GET", "/mtr-api/"+systemUUID+"/active-orders", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("unauthorized (session expired)")
	}
	var r ActiveOrdersResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return r.Orders, nil
}

func (c *Client) CreatePendingOrder(p PendingOrder, multiplier float64) error {
	vol, _ := strconv.ParseFloat(p.Volume, 64)
	sl, _ := strconv.ParseFloat(p.StopLoss, 64)
	tp, _ := strconv.ParseFloat(p.TakeProfit, 64)
	price, _ := strconv.ParseFloat(p.ActivationPrice, 64)
	vol = math.Round(vol*multiplier*100) / 100
	resp, err := c.do("POST", "/mtr-api/"+systemUUID+"/pending-order/create", CreatePendingOrderRequest{
		Instrument: p.Symbol, OrderSide: p.Side, Volume: vol,
		Type: p.Type, Price: price, SlPrice: sl, TpPrice: tp, IsMobile: false,
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

func (c *Client) CancelPendingOrder(p PendingOrder) error {
	resp, err := c.do("POST", "/mtr-api/"+systemUUID+"/pending-order/cancel", CancelPendingOrderRequest{
		ID: p.ID, Instrument: p.Symbol, OrderSide: p.Side, Type: p.Type,
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
	screenLogin      screen = iota // email + password
	screenAccounts                 // pick master then slave(s)
	screenMultiplier               // enter lot multiplier
	screenCopying                 // live copier
	screenEdit                    // modify slaves after setup
	screenSettings                // poll interval config
)

type pickStep int

const (
	pickMaster pickStep = iota
	pickSlave
)

// ────────────────────────────────────────────────────────────────────────────
// Model
// ────────────────────────────────────────────────────────────────────────────

type slaveClient struct {
	client   *Client
	config   SlaveConfig
	known    map[string]Position
	positions []Position
	balance  *BalanceResponse
	copied   int
	closed   int
	errors   int

	// Pending order tracking
	pendingKnown      map[string]PendingOrder
	pendingCopied     int
	pendingCancelled  int

	// Error tracking for recovery notifications
	hadError      bool   // true if previous poll had errors on this slave
}

type statsData struct {
	mu            sync.Mutex
	masterPos     []Position
	masterPending []PendingOrder
	masterBalance *BalanceResponse
	lastPoll      time.Time
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
	accounts      []APIAccount
	cursor        int
	masterID      string
	slaveID       string          // currently-being-picked slave (before multiplier set)
	pendingSlaves []SlaveConfig   // fully configured slaves (picked + multiplier set)
	pickStep      pickStep
	sharedJar     *Client         // holds the co-auth cookie for both logins

	// Multiplier screen
	multInput        textinput.Model
	multErr          string
	pendingSlaveMult float64
	editMode         bool // if true, multiplier screen returns to edit instead of accounts

	// Browser fingerprint (stable per install)
	browserID string

	// Settings
	pollMs       int           // poll interval in ms (from config or default)
	settingsInput textinput.Model

	// Account verification for edit "Add slave" — filters out expired accounts
	verifyingAccts bool
	verifiedAccts  map[string]bool

	// Notifications
	notifier *Notifier

	// Copier
	master        *Client
	masterHadError bool       // tracks master error state for recovery notification
	slaves        []*slaveClient
	stats          statsData
	logs           []LogEntry
	known          map[string]Position
	paused         bool

	// Pending order tracking (master side)
	pendingKnown map[string]PendingOrder
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

	setting := textinput.New()
	setting.Placeholder = "500"
	setting.Width = 10
	setting.CharLimit = 5

	m := Model{
		screen:        screenLogin,
		spinner:       sp,
		emailInput:    email,
		passInput:     pass,
		multInput:     mult,
		settingsInput: setting,
		known:         make(map[string]Position),
		pendingKnown:  make(map[string]PendingOrder),
		pendingSlaves: nil,
		pollMs:        defaultPollMs,
		notifier:      NewNotifier(),
	}

	// Pre-fill from saved config if available
	if cfg := loadConfig(); cfg != nil {
		m.emailInput.SetValue(cfg.Email)
		m.passInput.SetValue(cfg.Password)
		if len(cfg.Slaves) > 0 {
			m.multInput.SetValue(fmt.Sprintf("%.2f", cfg.Slaves[0].Multiplier))
		}
		// Generate stable browserId on first ever run
		if cfg.BrowserID == "" {
			cfg.BrowserID = newUUIDv4()
			saveConfig(*cfg)
		}
		m.browserID = cfg.BrowserID
		if cfg.PollMs > 0 {
			m.pollMs = cfg.PollMs
		}
	} else {
		// No config yet — generate browserId so one exists from the start
		id := newUUIDv4()
		m.browserID = id
		saveConfig(SavedConfig{BrowserID: id, PollMs: defaultPollMs})
	}

	return m
}

// ────────────────────────────────────────────────────────────────────────────
// Messages
// ────────────────────────────────────────────────────────────────────────────

type msgLoginDone     struct{ accounts []APIAccount; err error }
type msgSeedDone      struct{ count int; err error }
type msgPollTick      struct{}
type msgEditApplied   struct{ err error }
type msgAcctsVerified struct{ valid map[string]bool }

// ────────────────────────────────────────────────────────────────────────────
// Init
// ────────────────────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, textinput.Blink)
}

// contentOffset returns the estimated (top, left) centering offset for a block
// of the given width and height, based on the current terminal dimensions.
func (m Model) contentOffset(contentW, contentH int) (int, int) {
	top := 0
	if m.height > contentH {
		top = (m.height - contentH) / 2
	}
	left := 0
	if m.width > contentW {
		left = (m.width - contentW) / 2
	}
	return top, left
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

	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			return m.handleClick(msg.X, msg.Y)
		}

	case tea.KeyMsg:
		switch m.screen {
		case screenLogin:
			return m.handleLoginKey(msg)
		case screenAccounts:
			return m.handleAccountsKey(msg)
		case screenMultiplier:
			return m.handleMultiplierKey(msg)
		case screenEdit:
			return m.handleEditKey(msg)
		case screenSettings:
			return m.handleSettingsKey(msg)
		case screenCopying:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "l":
				return m.logout()
			case "p":
				m.paused = !m.paused
				if m.paused {
					m.log(LogInfo, "Copier paused")
					m.notifier.Send("copier_paused", NotifyInfo, "Copier Paused", "Trade copying has been paused")
				} else {
					m.log(LogOK, "Copier resumed")
					m.notifier.Send("copier_resumed", NotifyInfo, "Copier Resumed", "Trade copying has been resumed")
				}
			case "e":
				// Enter edit screen — copy current slaves into pending list
				m.pendingSlaves = make([]SlaveConfig, len(m.slaves))
				for i, sc := range m.slaves {
					m.pendingSlaves[i] = sc.config
				}
				m.screen = screenEdit
				m.cursor = 0
			case "s":
				m.settingsInput.SetValue(fmt.Sprintf("%d", m.pollMs))
				m.settingsInput.Focus()
				m.screen = screenSettings
			}
		}

	case msgLoginDone:
		m.connecting = false
		if msg.err != nil {
			m.loginErr = msg.err.Error()
		} else {
			m.accounts = msg.accounts
			// If saved config exists, verify accounts still valid → skip screens
			if cfg := loadConfig(); cfg != nil && cfg.MasterID != "" && len(cfg.Slaves) > 0 {
				m.masterID = cfg.MasterID
				m.pendingSlaves = cfg.Slaves

				// Verify master still exists in the accounts list
				masterValid := false
				for _, a := range msg.accounts {
					if a.TradingAccountID == cfg.MasterID {
						masterValid = true
						break
					}
				}
				// Verify all slaves still exist
				slavesValid := true
				for _, s := range cfg.Slaves {
					found := false
					for _, a := range msg.accounts {
						if a.TradingAccountID == s.AccountID {
							found = true
							break
						}
					}
					if !found {
						slavesValid = false
						break
					}
				}

				if masterValid && slavesValid {
					// All saved accounts are still valid → skip to copying
					return m.startCopier()
				}
				// Some accounts no longer exist → fall through to manual selection
				m.log(LogErr, "Saved config has invalid accounts — please re-select")
			}
			m.screen = screenAccounts
			m.cursor = 0
		}

	case msgSeedDone:
		m.screen = screenCopying
		var slaveIDs []string
		for _, sc := range m.slaves {
			slaveIDs = append(slaveIDs, sc.client.accountID)
		}
		m.log(LogOK, fmt.Sprintf("Authenticated — master #%s → %d slave(s): %s",
			m.master.accountID, len(m.slaves), strings.Join(slaveIDs, ", ")))
		if msg.err != nil {
			m.log(LogErr, "Seed error: "+msg.err.Error())
		} else {
			m.log(LogInfo, fmt.Sprintf("Ready — %d existing position(s) seeded (not copied)", msg.count))
		}
		cmds = append(cmds, m.cmdSchedulePoll())

	case msgPollTick:
		m.doPoll()
		cmds = append(cmds, m.cmdSchedulePoll())

	case msgEditApplied:
		m.screen = screenCopying
		m.paused = false
		if msg.err != nil {
			m.log(LogErr, msg.err.Error())
		}
		cmds = append(cmds, m.cmdSchedulePoll())

	case msgAcctsVerified:
		m.verifyingAccts = false
		m.verifiedAccts = msg.valid
		m.screen = screenAccounts
		m.cursor = 0
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

// handleAccountsEnter is the Enter handler, extracted for reuse by mouse clicks.
func (m Model) handleAccountsEnter() (tea.Model, tea.Cmd) {
	if m.pickStep == pickMaster {
		selected := m.accounts[m.cursor].TradingAccountID
		m.masterID = selected
		m.pickStep = pickSlave
		m.cursor = 0
		if m.cursor < len(m.accounts) && m.accounts[m.cursor].TradingAccountID == m.masterID && len(m.accounts) > 1 {
			m.cursor = 1
		}
	} else if m.editMode {
		// In edit mode, add the selected account and return
		if m.cursor < len(m.accounts) {
			selected := m.accounts[m.cursor].TradingAccountID
			for _, ps := range m.pendingSlaves {
				if ps.AccountID == selected || selected == m.masterID {
					return m, nil
				}
			}
			m.slaveID = selected
			m.multInput.SetValue("1.0")
			m.multErr = ""
			m.screen = screenMultiplier
			m.multInput.Focus()
		}
	} else {
		// "Done" option selected → go directly to startCopier
		if m.cursor == len(m.accounts) {
			if len(m.pendingSlaves) == 0 {
				return m, nil
			}
			return m.startCopier()
		}
		// Selecting a slave account
		selected := m.accounts[m.cursor].TradingAccountID
		if selected == m.masterID {
			return m, nil
		}
		for _, ps := range m.pendingSlaves {
			if ps.AccountID == selected {
				return m, nil
			}
		}
		m.slaveID = selected
		m.multInput.SetValue("1.0")
		m.multErr = ""
		m.screen = screenMultiplier
		m.multInput.Focus()
	}
	return m, nil
}

// isAccountDisabled returns true if the account at index i should be skipped
// during slave picking (it is either the master or already selected as a slave).
func (m Model) isAccountDisabled(i int) bool {
	if m.pickStep != pickSlave || i >= len(m.accounts) {
		return false
	}
	acc := m.accounts[i]
	if acc.TradingAccountID == m.masterID {
		return true
	}
	for _, ps := range m.pendingSlaves {
		if ps.AccountID == acc.TradingAccountID {
			return true
		}
	}
	return false
}

func (m Model) handleAccountsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// In pickSlave mode, the cursor can go up to len(accounts) (the "Done" option)
	// except during edit mode where we only pick one slave at a time
	maxCursor := len(m.accounts) - 1
	if m.pickStep == pickSlave && !m.editMode {
		maxCursor = len(m.accounts) // extra slot for "Done"
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			orig := m.cursor
			m.cursor--
			// Skip disabled accounts during slave picking
			for m.cursor >= 0 && m.isAccountDisabled(m.cursor) {
				m.cursor--
			}
			if m.cursor < 0 {
				m.cursor = orig // nowhere to go, stay put
			}
		}
	case "down", "j":
		if m.cursor < maxCursor {
			orig := m.cursor
			m.cursor++
			// Skip disabled accounts during slave picking
			for m.cursor < len(m.accounts) && m.isAccountDisabled(m.cursor) {
				m.cursor++
			}
			if m.cursor > maxCursor {
				m.cursor = orig // nowhere to go, stay put
			}
		}
	case "enter", " ":
		return m.handleAccountsEnter()
	case "esc", "b":
		if m.editMode {
			// In edit mode → return to edit screen
			m.editMode = false
			m.screen = screenEdit
			return m, nil
		}
		if m.pickStep == pickSlave {
			if len(m.pendingSlaves) > 0 {
				// Pop the last pending slave
				m.pendingSlaves = m.pendingSlaves[:len(m.pendingSlaves)-1]
			} else {
				m.pickStep = pickMaster
				m.masterID = ""
			}
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
		m.slaveID = ""
		return m, nil
	case "enter":
		mult, err := strconv.ParseFloat(strings.TrimSpace(m.multInput.Value()), 64)
		if err != nil || mult <= 0 {
			m.multErr = "Must be a positive number (e.g. 1.0)"
			return m, nil
		}
		m.multErr = ""
		m.pendingSlaveMult = mult

		if m.editMode {
			// Called from edit → update multiplier or add new slave
			found := false
			for i := range m.pendingSlaves {
				if m.pendingSlaves[i].AccountID == m.slaveID {
					m.pendingSlaves[i].Multiplier = mult
					found = true
					break
				}
			}
			if !found {
				m.pendingSlaves = append(m.pendingSlaves, SlaveConfig{
					AccountID: m.slaveID, Multiplier: mult,
				})
			}
			m.slaveID = ""
			m.editMode = false
			m.screen = screenEdit
			return m, nil
		}
		if m.slaveID != "" {
			// Configuring a specific slave → append to pending and go back
			m.pendingSlaves = append(m.pendingSlaves, SlaveConfig{
				AccountID: m.slaveID, Multiplier: mult,
			})
			m.slaveID = ""
			m.screen = screenAccounts
			return m, nil
		}
		// "Done" selected → start the copier with all pending slaves
		return m.startCopier()
	}
	var cmd tea.Cmd
	m.multInput, cmd = m.multInput.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m Model) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxCursor := len(m.pendingSlaves) + 1 // +1 for "Add", +1 for "Apply"
	if maxCursor < 0 {
		maxCursor = 0
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < maxCursor {
			m.cursor++
		}
	case "r":
		// Remove selected slave
		if m.cursor < len(m.pendingSlaves) {
			m.pendingSlaves = append(m.pendingSlaves[:m.cursor], m.pendingSlaves[m.cursor+1:]...)
			if m.cursor >= len(m.pendingSlaves) && m.cursor > 0 {
				m.cursor--
			}
		}
	case "m":
		// Change multiplier of selected slave
		if m.cursor < len(m.pendingSlaves) {
			m.slaveID = m.pendingSlaves[m.cursor].AccountID
			m.multInput.SetValue(fmt.Sprintf("%.2f", m.pendingSlaves[m.cursor].Multiplier))
			m.multErr = ""
			m.editMode = true
			m.screen = screenMultiplier
			m.multInput.Focus()
		}
	case "enter", " ":
		// "Add slave" option — verify accounts first to filter out expired ones
		if m.cursor == len(m.pendingSlaves) {
			m.editMode = true
			m.pickStep = pickSlave
			m.slaveID = ""
			m.cursor = 0
			m.verifyingAccts = true
			m.verifiedAccts = nil
			return m, func() tea.Msg {
				valid := make(map[string]bool)
				for _, acc := range m.accounts {
					if acc.TradingAccountID == m.masterID {
						continue // skip master
					}
					isPicked := false
					for _, ps := range m.pendingSlaves {
						if ps.AccountID == acc.TradingAccountID {
							isPicked = true
							break
						}
					}
					if isPicked {
						continue // skip already-configured slaves
					}
					// Try to verify — use the token from the login response
					c := &Client{http: m.sharedJar.http, browserID: m.browserID}
					c.tradingApiToken = acc.TradingApiToken
					c.accountID = acc.TradingAccountID
					if _, err := c.GetBalance(); err == nil {
						valid[acc.TradingAccountID] = true
					}
				}
				return msgAcctsVerified{valid}
			}
		}
		// "Apply & restart" option
		if m.cursor == len(m.pendingSlaves)+1 {
			if len(m.pendingSlaves) == 0 {
				return m, nil
			}
			return m.applyEdit()
		}
	case "esc", "b":
		// Discard changes, back to copying — refresh immediately
		m.screen = screenCopying
		return m, m.cmdSchedulePoll()
	}
	return m, nil
}

func (m Model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc", "b":
		// Discard, back to copying — refresh immediately
		m.screen = screenCopying
		return m, m.cmdSchedulePoll()
	case "enter":
		val := strings.TrimSpace(m.settingsInput.Value())
		if val == "" {
			return m, nil
		}
		n, err := strconv.Atoi(val)
		if err != nil || n < 50 {
			m.settingsInput.SetValue(fmt.Sprintf("%d", m.pollMs))
			return m, nil
		}
		m.pollMs = n
		// Persist to config
		cfg := loadConfig()
		if cfg != nil {
			cfg.PollMs = n
			saveConfig(*cfg)
		} else {
			saveConfig(SavedConfig{PollMs: n, BrowserID: m.browserID})
		}
		m.screen = screenCopying
		return m, m.cmdSchedulePoll()
	default:
		m.settingsInput, _ = m.settingsInput.Update(msg)
		return m, nil
	}
}

// itemPos stores the Y-range of an item within a vertical layout.
type itemPos struct {
	startY int
	endY   int
}

// measureItems computes cumulative Y positions of items stacked vertically.
// Each item's height is measured with lipgloss.Height (matching JoinVertical).
func measureItems(items []string) ([]itemPos, int) {
	pos := make([]itemPos, len(items))
	y := 0
	for i, item := range items {
		h := lipgloss.Height(item)
		pos[i] = itemPos{startY: y, endY: y + h}
		y += h
	}
	return pos, y
}

// contentTop returns the Y offset of centered content within the terminal.
func (m Model) contentTop(totalH int) int {
	top, _ := m.contentOffset(m.contentW(), totalH)
	return top
}

// handleClick maps a mouse click (x, y) to the appropriate action based on
// the current screen. Y is 0-indexed from top of terminal.
// Positions are computed by building the exact same items as the View()
// and measuring each item's actual rendered height with lipgloss.Height.
func (m Model) handleClick(x, y int) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenLogin:
		return m.handleClickLogin(y)
	case screenAccounts:
		return m.handleClickAccounts(y)
	case screenEdit:
		return m.handleClickEdit(y)
	case screenMultiplier:
		return m.handleClickMultiplier(y)
	case screenCopying:
		return m.handleClickCopying(x, y)
	}
	return m, nil
}

func (m Model) handleClickLogin(y int) (tea.Model, tea.Cmd) {
	title := styleTitle.Render("  ◈  FundingPips Trade Copier  ")
	notice := ""
	if cfg := loadConfig(); cfg != nil {
		slaveCount := len(cfg.Slaves)
		suffix := fmt.Sprintf("master #%s → %d slave(s)", cfg.MasterID, slaveCount)
		notice = styleDim.Render("  Saved config found — " + suffix)
	}
	emailBox := styleInput.Render(m.emailInput.View())
	passBox := styleInput.Render(m.passInput.View())
	btn := styleBtn.Render("   Sign In   ")
	errLine := ""
	if m.loginErr != "" {
		errLine = "\n" + styleLogErr.Render("  ⚠  " + m.loginErr)
	}
	status := ""
	if m.connecting {
		status = "\n" + m.spinner.View() + styleBlue.Render("  Signing in...")
	}
	help := styleDim.Render("  Tab · ↑↓  navigate    Enter  confirm    Ctrl+C  quit")

	// Exact same items as viewLogin()
	items := []string{
		title, "",
		notice, "",
		styleLabel.Render("  Email"), emailBox,
		"",
		styleLabel.Render("  Password"), passBox,
		"",
		btn,
		errLine, status,
		"", help,
	}
	pos, totalH := measureItems(items)
	top := m.contentTop(totalH)
	relY := y - top
	if relY < 0 || relY >= totalH {
		return m, nil
	}

	for i, p := range pos {
		if p.startY <= relY && relY < p.endY {
			switch i {
			case 5: // emailBox
				m.loginFocus = 0
				m.emailInput.Focus()
				m.passInput.Blur()
			case 8: // passBox
				m.loginFocus = 1
				m.passInput.Focus()
				m.emailInput.Blur()
			case 10: // btn
				m.loginFocus = 2
				m.emailInput.Blur()
				m.passInput.Blur()
				return m.doLogin()
			}
			break
		}
	}
	return m, nil
}

func (m Model) handleClickAccounts(y int) (tea.Model, tea.Cmd) {
	title := styleTitle.Render("  ◈  Select Accounts  ")

	var stepLabel string
	if m.pickStep == pickMaster {
		stepLabel = styleMasterTag.Render(" MASTER ") + styleDim.Render("  Choose the account to copy FROM")
	} else {
		pendingStr := ""
		if len(m.pendingSlaves) > 0 {
			var ids []string
			for _, ps := range m.pendingSlaves {
				ids = append(ids, "#"+ps.AccountID+"×"+fmt.Sprintf("%.2f", ps.Multiplier))
			}
			pendingStr = "  " + styleDim.Render("| selected: ") + styleGreen.Render(strings.Join(ids, ", "))
		}
		stepLabel = styleSlaveTag.Render(" SLAVE ") + styleDim.Render("  Choose accounts to copy TO") +
			"  " + styleDim.Render("master: ") + styleBlue.Render("#"+m.masterID) + pendingStr
	}

	// If verifying accounts — no clickable items
	if m.verifyingAccts {
		return m, nil
	}

	// Build account rows (exact same as viewAccounts) and track clickable row → account index
	pickedIDs := make(map[string]bool)
	for _, ps := range m.pendingSlaves {
		pickedIDs[ps.AccountID] = true
	}

	rowW := m.contentW() - 6
	idW := 10
	curW := 8
	typeW := 6
	nameW := rowW - idW - curW - typeW - 8
	if nameW < 10 {
		idW = 6
		curW = 4
		nameW = rowW - idW - curW - typeW - 8
		if nameW < 5 {
			nameW = 5
		}
	}

	var rows []string
	var rowToAcctIdx []int // maps row index within panel content → account index
	for i, acc := range m.accounts {
		isMaster := acc.TradingAccountID == m.masterID
		isPicked := pickedIDs[acc.TradingAccountID]
		isDisabled := m.pickStep == pickSlave && (isMaster || isPicked)

		if m.verifiedAccts != nil && m.pickStep == pickSlave && !isMaster && !isPicked {
			if !m.verifiedAccts[acc.TradingAccountID] {
				continue
			}
		}

		name := acc.Offer.Description
		if name == "" {
			name = acc.Offer.Name
		}
		acctType := "DEMO"
		if !acc.Offer.Demo {
			acctType = "LIVE"
		}

		if isMaster && m.pickStep == pickSlave {
			line := fmt.Sprintf("  ▶  #%-*s  %-*s  %-*s  [%s]",
				idW, truncate(acc.TradingAccountID, idW),
				curW, truncate(acc.Offer.Currency, curW),
				nameW, truncate(name, nameW),
				acctType)
			rows = append(rows, styleDim.Render(line)+"  "+styleMasterTag.Render(" SOURCE "))
			rowToAcctIdx = append(rowToAcctIdx, -1) // not clickable (master header)
			continue
		}

		if isDisabled {
			continue
		}

		line := fmt.Sprintf("  #%-*s  %-*s  %-*s  [%s]",
			idW, truncate(acc.TradingAccountID, idW),
			curW, truncate(acc.Offer.Currency, curW),
			nameW, truncate(name, nameW),
			acctType,
		)
		var row string
		if i == m.cursor {
			row = styleAccountRowFocused.Render(line)
		} else {
			row = styleAccountRow.Render(line)
		}
		rows = append(rows, row)
		rowToAcctIdx = append(rowToAcctIdx, i)
	}

	if m.pickStep == pickSlave && !m.editMode {
		doneLabel := "  ✓  Done selecting slaves"
		if m.cursor == len(m.accounts) {
			rows = append(rows, styleAccountRowFocused.Render(doneLabel))
		} else {
			rows = append(rows, styleAccountRow.Render(doneLabel))
		}
		rowToAcctIdx = append(rowToAcctIdx, -2) // -2 = Done button
	}

	list := stylePanel.Render(strings.Join(rows, "\n"))
	help := styleDim.Render("  ↑↓ / j k  navigate    Enter / Space  select    Esc / b  back    q  quit")

	// Items: [title, "", stepLabel, "", list, "", help]
	items := []string{title, "", stepLabel, "", list, "", help}
	pos, totalH := measureItems(items)
	top := m.contentTop(totalH)
	relY := y - top
	if relY < 0 || relY >= totalH {
		return m, nil
	}

	// item 3 = 'list' (the panel)
	if pos[3].startY <= relY && relY < pos[3].endY {
		// Inside the panel: skip top border (1 line)
		rowIdx := relY - pos[3].startY - 1
		if rowIdx >= 0 && rowIdx < len(rowToAcctIdx) {
			acctIdx := rowToAcctIdx[rowIdx]
			if acctIdx == -2 {
				// Done button
				m.cursor = len(m.accounts)
				return m.handleAccountsEnter()
			}
			if acctIdx >= 0 {
				m.cursor = acctIdx
				return m.handleAccountsEnter()
			}
			// acctIdx == -1 is master header — no action
		}
	}
	return m, nil
}

func (m Model) handleClickEdit(y int) (tea.Model, tea.Cmd) {
	title := styleTitle.Render("  ◈  Edit Configuration  ")

	var rows []string
	maxW := m.contentW() - 6
	masterLine := styleBlue.Render("⬟ MASTER") + "  " + styleLabel.Render("#" + m.masterID)
	rows = append(rows, "  "+truncate(masterLine, maxW), "")

	nSlaves := len(m.pendingSlaves)
	if nSlaves == 0 {
		rows = append(rows, styleDim.Render("  — no slaves configured —"))
	} else {
		for i, ps := range m.pendingSlaves {
			line := fmt.Sprintf("  SLAVE %d:  #%-*s  ×%.2f", i+1,
				min(10, maxW-20), truncate(ps.AccountID, min(10, maxW-20)),
				ps.Multiplier)
			if i == m.cursor {
				rows = append(rows, styleAccountRowFocused.Render(line))
			} else {
				rows = append(rows, styleAccountRow.Render(line))
			}
		}
	}

	addLine := "  [+] Add slave"
	if m.cursor == nSlaves {
		rows = append(rows, styleAccountRowFocused.Render(addLine))
	} else {
		rows = append(rows, styleAccountRow.Render(addLine))
	}
	applyLine := "  [>] Apply & restart copier"
	if m.cursor == nSlaves+1 {
		rows = append(rows, styleAccountRowFocused.Render(applyLine))
	} else {
		rows = append(rows, styleAccountRow.Render(applyLine))
	}

	list := stylePanel.Render(strings.Join(rows, "\n"))
	help := styleDim.Render("  ↑↓  navigate    r  remove slave    m  change multiplier    Enter  confirm    Esc  back    q  quit")

	// Items: [title, "", list, "", help]
	items := []string{title, "", list, "", help}
	pos, totalH := measureItems(items)
	top := m.contentTop(totalH)
	relY := y - top
	if relY < 0 || relY >= totalH {
		return m, nil
	}

	// item 2 = 'list' (the panel)
	if pos[2].startY <= relY && relY < pos[2].endY {
		// Inside panel: skip top border (1 line)
		rowIdx := relY - pos[2].startY - 1 // index into rows[]
		if rowIdx < 0 || rowIdx >= len(rows) {
			return m, nil
		}

		// rows layout: [master(0), empty(1), slave rows or no-slaves(2..), add, apply]
		// When nSlaves==0, a "no slaves" text occupies rows[2], shifting add/apply
		noSlavesShift := 0
		if nSlaves == 0 {
			noSlavesShift = 1
		}

		if rowIdx >= 2 && rowIdx < 2+nSlaves {
			// Slave row
			m.cursor = rowIdx - 2
			return m, nil
		}
		if rowIdx == 2+nSlaves+noSlavesShift {
			// Add slave
			m.editMode = true
			m.pickStep = pickSlave
			m.slaveID = ""
			m.cursor = 0
			m.screen = screenAccounts
			return m, nil
		}
		if rowIdx == 2+nSlaves+noSlavesShift+1 {
			// Apply & restart
			if nSlaves > 0 {
				return m.applyEdit()
			}
		}
	}
	return m, nil
}

func (m Model) handleClickMultiplier(y int) (tea.Model, tea.Cmd) {
	title := styleTitle.Render("  ◈  Lot Multiplier  ")

	var summary string
	var btn string
	if m.slaveID != "" {
		summary = lipgloss.JoinHorizontal(lipgloss.Top,
			styleDim.Render("Master  "), styleMasterTag.Render(" #"+m.masterID+" "),
			styleDim.Render("  →  "),
			styleSlaveTag.Render(" #"+m.slaveID+" "), styleDim.Render("  Slave"),
		)
		btn = styleBtnFocused.Render(" ▶  Add Slave  ")
	} else {
		var pendingStr string
		if len(m.pendingSlaves) > 0 {
			var ids []string
			for _, ps := range m.pendingSlaves {
				ids = append(ids, "#"+ps.AccountID+"×"+fmt.Sprintf("%.2f", ps.Multiplier))
			}
			pendingStr = styleGreen.Render(strings.Join(ids, ", "))
		}
		summary = lipgloss.JoinHorizontal(lipgloss.Top,
			styleDim.Render("Master  "), styleMasterTag.Render(" #"+m.masterID+" "),
			styleDim.Render("  →  "),
			styleSlaveTag.Render(fmt.Sprintf(" %d slave(s) ", len(m.pendingSlaves))),
			"  "+pendingStr,
		)
		btn = styleBtnFocused.Render(" ▶  Start Copier  ")
	}

	desc := styleDim.Render("  Scale slave lot size relative to master.\n  1.0 = same size  ·  0.5 = half  ·  2.0 = double")
	box := styleFocused.Render(m.multInput.View())
	errLine := ""
	if m.multErr != "" {
		errLine = "\n" + styleLogErr.Render("  ⚠  " + m.multErr)
	}
	help := styleDim.Render("  Enter  confirm    Esc / b  back    Ctrl+C  quit")

	// Same items as viewMultiplier
	items := []string{
		title, "",
		summary, "",
		desc, "",
		box,
		errLine, "",
		btn,
		"", help,
	}
	pos, totalH := measureItems(items)
	top := m.contentTop(totalH)
	relY := y - top
	if relY < 0 || relY >= totalH {
		return m, nil
	}

	// item 6 = box (multiplier input), item 9 = btn
	for i, p := range pos {
		if p.startY <= relY && relY < p.endY {
			switch i {
			case 6: // box
				m.multInput.Focus()
			case 9: // btn — same as pressing Enter
				mult, err := strconv.ParseFloat(strings.TrimSpace(m.multInput.Value()), 64)
				if err != nil || mult <= 0 {
					m.multErr = "Must be a positive number (e.g. 1.0)"
					return m, nil
				}
				m.multErr = ""
				m.pendingSlaveMult = mult

				if m.editMode {
					found := false
					for i := range m.pendingSlaves {
						if m.pendingSlaves[i].AccountID == m.slaveID {
							m.pendingSlaves[i].Multiplier = mult
							found = true
							break
						}
					}
					if !found {
						m.pendingSlaves = append(m.pendingSlaves, SlaveConfig{
							AccountID: m.slaveID, Multiplier: mult,
						})
					}
					m.slaveID = ""
					m.editMode = false
					m.screen = screenEdit
					return m, nil
				}
				if m.slaveID != "" {
					m.pendingSlaves = append(m.pendingSlaves, SlaveConfig{
						AccountID: m.slaveID, Multiplier: mult,
					})
					m.slaveID = ""
					m.screen = screenAccounts
					return m, nil
				}
				return m.startCopier()
			}
			break
		}
	}
	return m, nil
}

func (m Model) handleClickCopying(x, y int) (tea.Model, tea.Cmd) {
	// Render full copying view to measure body height
	body := m.viewCopying()
	bodyH := lipgloss.Height(body)

	// Footer is the last line of the body
	if y != bodyH-1 {
		return m, nil
	}

	// Build footer buttons (same as viewCopying) to measure individual widths
	var btnStrs []string
	if m.paused {
		btnStrs = append(btnStrs, styleFooterBtnAccent.Render(" ▶ p Resume "))
	} else {
		btnStrs = append(btnStrs, styleFooterBtn.Render(" ⏸ p Pause "))
	}
	btnStrs = append(btnStrs, styleFooterBtn.Render(" ⚙ s Settings "))
	btnStrs = append(btnStrs, styleFooterBtn.Render(" ✎ e Edit "))
	btnStrs = append(btnStrs, styleFooterBtn.Render(" ⇥ l Logout "))
	btnStrs = append(btnStrs, styleFooterBtn.Render(" ✕ q Quit "))

	runningX := 0
	for idx, btn := range btnStrs {
		btnW := lipgloss.Width(btn)
		if x >= runningX && x < runningX+btnW {
			switch idx {
			case 0: // Pause/Resume
				m.paused = !m.paused
				if m.paused {
					m.log(LogInfo, "Copier paused")
					m.notifier.Send("copier_paused", NotifyInfo, "Copier Paused", "Trade copying has been paused")
				} else {
					m.log(LogOK, "Copier resumed")
					m.notifier.Send("copier_resumed", NotifyInfo, "Copier Resumed", "Trade copying has been resumed")
				}
			case 1: // Settings
				m.settingsInput.SetValue(fmt.Sprintf("%d", m.pollMs))
				m.settingsInput.Focus()
				m.screen = screenSettings
			case 2: // Edit
				m.pendingSlaves = make([]SlaveConfig, len(m.slaves))
				for i, sc := range m.slaves {
					m.pendingSlaves[i] = sc.config
				}
				m.screen = screenEdit
				m.cursor = 0
			case 3: // Logout
				return m.logout()
			case 4: // Quit
				return m, tea.Quit
			}
			break
		}
		runningX += btnW
	}
	return m, nil
}

func (m Model) applyEdit() (tea.Model, tea.Cmd) {
	email := strings.TrimSpace(m.emailInput.Value())

	// Save updated config
	saveConfig(SavedConfig{
		Email:     email,
		Password:  m.passInput.Value(),
		MasterID:  m.masterID,
		Slaves:    m.pendingSlaves,
		BrowserID: m.browserID,
		PollMs:    m.pollMs,
	})

	// Rebuild slave clients
	// For existing slaves, keep the client (no re-login needed)
	// For new slaves, create new clients (they will be logged in below)
	oldByID := make(map[string]*slaveClient)
	for _, sc := range m.slaves {
		oldByID[sc.config.AccountID] = sc
	}

	newSlaves := make([]*slaveClient, 0, len(m.pendingSlaves))
	var needLogin []SlaveConfig
	for _, cfg := range m.pendingSlaves {
		if sc, ok := oldByID[cfg.AccountID]; ok {
			// Update multiplier on existing slave
			sc.config.Multiplier = cfg.Multiplier
			newSlaves = append(newSlaves, sc)
		} else {
			needLogin = append(needLogin, cfg)
		}
	}
	m.slaves = newSlaves

	if len(needLogin) == 0 {
		m.screen = screenCopying
		m.log(LogOK, "Slave configuration updated")
		m.notifier.Send("config_updated", NotifyInfo,
			"Configuration Updated",
			fmt.Sprintf("Multipliers applied to %d slave(s)", len(m.slaves)))
		return m, m.cmdSchedulePoll()
	}

	// Log in new slaves in background
	pass := m.passInput.Value()
	return m, func() tea.Msg {
		var errs []string
		for _, cfg := range needLogin {
			sc := &slaveClient{
				client: &Client{http: m.sharedJar.http},
				config: cfg,
				known:  make(map[string]Position),
			}
			slaveAccounts, err := sc.client.LoginAll(email, pass)
			if err != nil {
				errs = append(errs, fmt.Sprintf("slave #%s login: %v", cfg.AccountID, err))
				continue
			}
			if err := sc.client.SelectAccount(slaveAccounts, cfg.AccountID); err != nil {
				errs = append(errs, fmt.Sprintf("slave #%s select: %v", cfg.AccountID, err))
				continue
			}
			// Seed known positions from master
			for id, pos := range m.known {
				sc.known[id] = pos
			}
			m.slaves = append(m.slaves, sc)
		}
		errStr := ""
		changeMsg := "Multipliers applied"
		if len(needLogin) > 0 {
			changeMsg = fmt.Sprintf("Multipliers applied + %d new slave(s)", len(needLogin))
		}
		if len(errs) > 0 {
			errStr = strings.Join(errs, "; ")
			m.log(LogErr, "Edit apply: "+errStr)
		}
		m.log(LogOK, "Slave configuration updated")
		m.notifier.Send("config_updated", NotifyInfo,
			"Configuration Updated", changeMsg)
		return msgEditApplied{err: nil}
	}
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
	m.sharedJar.browserID = m.browserID

	return m, func() tea.Msg {
		accounts, err := m.sharedJar.LoginAll(email, pass)
		return msgLoginDone{accounts, err}
	}
}

func (m Model) startCopier() (tea.Model, tea.Cmd) {
	email := strings.TrimSpace(m.emailInput.Value())

	// Save config with all pending slaves
	slaves := m.pendingSlaves
	if len(slaves) == 0 {
		// Fallback: should not happen (Done requires ≥1 slave)
		slaves = []SlaveConfig{{AccountID: m.slaveID, Multiplier: m.pendingSlaveMult}}
	}
	saveConfig(SavedConfig{
		Email:     email,
		Password:  m.passInput.Value(),
		MasterID:  m.masterID,
		Slaves:    slaves,
		BrowserID: m.browserID,
		PollMs:    m.pollMs,
	})

	// Master uses the shared jar (co-auth session)
	m.master = &Client{http: m.sharedJar.http, browserID: m.browserID}
	m.paused = false
	pass := m.passInput.Value()

	// Slave clients will be built only for successful logins inside the closure
	m.slaves = nil

	return m, func() tea.Msg {
		// Select master from already-logged-in accounts
		if err := m.master.SelectAccount(m.accounts, m.masterID); err != nil {
			return msgLoginDone{err: fmt.Errorf("master: %w", err)}
		}

		// Log in all slaves — only keep those that succeed
		var slaveErrs []string
		var validSlaves []*slaveClient
		for _, s := range slaves {
			sc := &slaveClient{
				client:       &Client{http: m.sharedJar.http, browserID: m.browserID},
				config:       s,
				known:        make(map[string]Position),
				pendingKnown: make(map[string]PendingOrder),
			}
			slaveAccounts, err := sc.client.LoginAll(email, pass)
			if err != nil {
				msg := fmt.Sprintf("slave #%s login: %v", s.AccountID, err)
				slaveErrs = append(slaveErrs, msg)
				m.notifier.Send("slave_auth_"+s.AccountID, NotifyCritical,
					"Slave Auth Failed",
					fmt.Sprintf("Account #%s: %s", s.AccountID, err.Error()))
				continue
			}
			if err := sc.client.SelectAccount(slaveAccounts, s.AccountID); err != nil {
				slaveErrs = append(slaveErrs, fmt.Sprintf("slave #%s select: %v", s.AccountID, err))
				m.notifier.Send("slave_auth_"+s.AccountID, NotifyCritical,
					"Slave Auth Failed",
					fmt.Sprintf("Account #%s: %s", s.AccountID, err.Error()))
				continue
			}
			validSlaves = append(validSlaves, sc)
		}
		m.slaves = validSlaves

		// Seed existing master positions — shared across all slaves
		pos, err := m.master.GetOpenPositions()
		if err != nil {
			if len(slaveErrs) > 0 {
				return msgSeedDone{err: fmt.Errorf("seed: %v; slave errors: %s", err, strings.Join(slaveErrs, "; "))}
			}
			return msgSeedDone{err: err}
		}
		for _, p := range pos {
			m.known[p.ID] = p
			for _, sc := range m.slaves {
				sc.known[p.ID] = p
			}
		}

		// Seed existing master pending orders — so we don't re-copy them
		pendingOrders, err := m.master.GetActiveOrders()
		if err == nil {
			for _, po := range pendingOrders {
				m.pendingKnown[po.ID] = po
				for _, sc := range m.slaves {
					sc.pendingKnown[po.ID] = po
				}
			}
		}

		// Populate stats immediately so the first render shows real data
		if masterBal, err := m.master.GetBalance(); err == nil {
			m.stats.masterBalance = masterBal
		}
		m.stats.masterPos = pos
		if err == nil {
			m.stats.masterPending = pendingOrders
		}
		m.stats.lastPoll = time.Now()
		// Pre-fetch slave balances too
		for _, sc := range m.slaves {
			bal, err := sc.client.GetBalance()
			if err == nil {
				sc.balance = bal
			}
			spos, _ := sc.client.GetOpenPositions()
			sc.positions = spos
		}

		if len(slaveErrs) > 0 {
			errMsg := fmt.Sprintf("slave errors: %s", strings.Join(slaveErrs, "; "))
			if len(m.slaves) == 0 {
				// All slaves failed — don't start the copier
				return msgLoginDone{err: fmt.Errorf("all slaves failed — %s", errMsg)}
			}
			return msgSeedDone{err: fmt.Errorf(errMsg)}
		}
		m.notifier.Send("copy_started", NotifyInfo,
			"Copy Started",
			fmt.Sprintf("Master #%s → %d slave(s)", m.masterID, len(m.slaves)))
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
	ms := m.pollMs
	if ms < 50 {
		ms = 50 // hard floor to avoid actual spam
	}
	return tea.Tick(time.Duration(ms)*time.Millisecond, func(time.Time) tea.Msg {
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

func pendingMap(orders []PendingOrder) map[string]PendingOrder {
	m := make(map[string]PendingOrder, len(orders))
	for _, po := range orders {
		m[po.ID] = po
	}
	return m
}

func (m *Model) doPoll() {
	if m.screen != screenCopying || m.paused {
		return
	}

	// ── Master positions ────────────────────────────────────────────────
	masterHadErrorBefore := m.masterHadError
	masterPos, err := m.master.GetOpenPositions()
	if err != nil {
		m.log(LogErr, "Poll: "+err.Error())
		m.masterHadError = true
		if !masterHadErrorBefore {
			m.notifier.Send("master_disconnect", NotifyCritical,
				"Master Disconnected",
				"Cannot fetch master positions: "+err.Error())
		}
		return
	}
	m.masterHadError = false
	if masterHadErrorBefore {
		m.notifier.Send("master_recovered", NotifyWarning,
			"Master Recovered",
			"Master connection restored")
	}

	current := posMap(masterPos)

	// For each master position change, apply to ALL slaves
	for id, pos := range current {
		if _, known := m.known[id]; !known {
			side := styleGreen.Render(pos.Side)
			if pos.Side == "SELL" {
				side = styleRed.Render(pos.Side)
			}
			m.log(LogTrade, fmt.Sprintf("NEW %s  %-8s  vol=%-6s  @ %s",
				side, pos.Symbol, pos.Volume, pos.OpenPrice))

			for _, sc := range m.slaves {
				mult := sc.config.Multiplier
				if err := sc.client.OpenPosition(pos, mult); err != nil {
					sc.errors++
					m.log(LogErr, fmt.Sprintf("  ↳ slave #%s open failed: %s", sc.client.accountID, err.Error()))
					notifyKey := "trade_reject_" + sc.config.AccountID + "_" + pos.Symbol
					if isRateLimit(err) {
						m.notifier.Send("rate_limit", NotifyWarning,
							"Rate Limited",
							err.Error())
					} else {
						m.notifier.Send(notifyKey, NotifyCritical,
							"Trade Rejected",
							fmt.Sprintf("Slave #%s  %s %s: %s", sc.config.AccountID, pos.Side, pos.Symbol, err.Error()))
					}
				} else {
					sc.copied++
					scaledVol := pos.Volume
					if v, e2 := strconv.ParseFloat(pos.Volume, 64); e2 == nil {
						scaledVol = fmt.Sprintf("%.2f", math.Round(v*mult*100)/100)
					}
					m.log(LogOK, fmt.Sprintf("  ↳ slave #%s opened  vol=%s ✓", sc.client.accountID, scaledVol))
				}
			}
		}
	}

	// Closed → close on all slaves
	for id, pos := range m.known {
		if _, stillOpen := current[id]; !stillOpen {
			m.log(LogTrade, fmt.Sprintf("CLOSED %-6s %-8s → closing on slaves...", pos.Side, pos.Symbol))
			for _, sc := range m.slaves {
				slavePos, err := sc.client.GetOpenPositions()
				if err != nil {
					sc.errors++
					m.log(LogErr, fmt.Sprintf("  ↳ slave #%s: could not fetch positions: %s", sc.client.accountID, err.Error()))
					if isRateLimit(err) {
						m.notifier.Send("rate_limit", NotifyWarning,
							"Rate Limited",
							err.Error())
					}
					continue
				}
				found := false
				for _, sp := range slavePos {
					if sp.Symbol == pos.Symbol && sp.Side == pos.Side {
						if err := sc.client.ClosePosition(sp); err != nil {
							sc.errors++
							m.log(LogErr, fmt.Sprintf("  ↳ slave #%s close failed: %s", sc.client.accountID, err.Error()))
							notifyKey := "close_reject_" + sc.config.AccountID + "_" + pos.Symbol
							if isRateLimit(err) {
								m.notifier.Send("rate_limit", NotifyWarning,
									"Rate Limited",
									err.Error())
							} else {
								m.notifier.Send(notifyKey, NotifyCritical,
									"Close Rejected",
									fmt.Sprintf("Slave #%s  %s %s: %s", sc.config.AccountID, pos.Side, pos.Symbol, err.Error()))
							}
						} else {
							sc.closed++
							m.log(LogOK, fmt.Sprintf("  ↳ slave #%s closed ✓  P&L was %s", sc.client.accountID, sp.Profit))
							found = true
						}
						break
					}
				}
				if !found {
					m.log(LogErr, fmt.Sprintf("  ↳ slave #%s: no matching position for %s %s", sc.client.accountID, pos.Side, pos.Symbol))
					m.notifier.Send("sync_mismatch_"+sc.config.AccountID+"_"+pos.Symbol, NotifyWarning,
						"Position Sync Mismatch",
						fmt.Sprintf("Slave #%s has no position matching %s %s", sc.config.AccountID, pos.Side, pos.Symbol))
				}
			}
		}
	}

	m.known = current

	// ── Pending order sync ──────────────────────────────────────────────
	masterPending, err := m.master.GetActiveOrders()
	if err != nil {
		m.log(LogErr, "Pending poll: "+err.Error())
		if !masterHadErrorBefore && m.masterHadError {
			// Error is new this poll (master is having trouble)
		}
	} else {
		currentPending := pendingMap(masterPending)

		// New pending orders on master → create on all slaves
		for id, po := range currentPending {
			if _, known := m.pendingKnown[id]; !known {
				side := styleGreen.Render(po.Side)
				if po.Side == "SELL" {
					side = styleRed.Render(po.Side)
				}
				m.log(LogTrade, fmt.Sprintf("NEW PENDING %s %s  %-7s  vol=%-6s  @ %s",
					po.Type, side, po.Symbol, po.Volume, po.ActivationPrice))

				for _, sc := range m.slaves {
					mult := sc.config.Multiplier
					if err := sc.client.CreatePendingOrder(po, mult); err != nil {
						sc.errors++
						m.log(LogErr, fmt.Sprintf("  ↳ slave #%s pending create failed: %s", sc.client.accountID, err.Error()))
						notifyKey := "order_reject_" + sc.config.AccountID + "_" + po.Symbol
						if isRateLimit(err) {
							m.notifier.Send("rate_limit", NotifyWarning,
								"Rate Limited",
								err.Error())
						} else {
							m.notifier.Send(notifyKey, NotifyCritical,
								"Pending Order Rejected",
								fmt.Sprintf("Slave #%s  %s %s %s: %s",
									sc.config.AccountID, po.Type, po.Side, po.Symbol, err.Error()))
						}
					} else {
						sc.pendingCopied++
						scaledVol := po.Volume
						if v, e2 := strconv.ParseFloat(po.Volume, 64); e2 == nil {
							scaledVol = fmt.Sprintf("%.2f", math.Round(v*mult*100)/100)
						}
						m.log(LogOK, fmt.Sprintf("  ↳ slave #%s pending created  vol=%s ✓", sc.client.accountID, scaledVol))
					}
				}
			}
		}

		// Cancelled/filled pending orders → cancel on all slaves
		for id, po := range m.pendingKnown {
			if _, stillActive := currentPending[id]; !stillActive {
				m.log(LogTrade, fmt.Sprintf("REMOVED PENDING %s %-6s %-8s → cancelling on slaves...",
					po.Type, po.Side, po.Symbol))
				for _, sc := range m.slaves {
					if err := sc.client.CancelPendingOrder(po); err != nil {
						sc.errors++
						m.log(LogErr, fmt.Sprintf("  ↳ slave #%s cancel pending failed: %s", sc.client.accountID, err.Error()))
						notifyKey := "cancel_reject_" + sc.config.AccountID + "_" + po.Symbol
						if isRateLimit(err) {
							m.notifier.Send("rate_limit", NotifyWarning,
								"Rate Limited",
								err.Error())
						} else {
							m.notifier.Send(notifyKey, NotifyCritical,
								"Cancel Rejected",
								fmt.Sprintf("Slave #%s  %s %s %s: %s",
									sc.config.AccountID, po.Type, po.Side, po.Symbol, err.Error()))
						}
					} else {
						sc.pendingCancelled++
						m.log(LogOK, fmt.Sprintf("  ↳ slave #%s pending cancelled ✓", sc.client.accountID))
					}
				}
			}
		}

		m.pendingKnown = currentPending
	}

	// ── Slave balances & recovery detection ─────────────────────────────
	masterBal, _ := m.master.GetBalance()
	for _, sc := range m.slaves {
		hadErrorBefore := sc.hadError
		pollHadError := false

		bal, err := sc.client.GetBalance()
		if err != nil {
			pollHadError = true
			if isRateLimit(err) {
				m.notifier.Send("rate_limit", NotifyWarning,
					"Rate Limited",
					err.Error())
			}
			// Check for auth failure
			if strings.Contains(err.Error(), "unauthorized") || strings.Contains(err.Error(), "401") {
				m.notifier.Send("slave_auth_"+sc.config.AccountID, NotifyCritical,
					"Slave Auth Failed",
					fmt.Sprintf("Account #%s: %s", sc.config.AccountID, err.Error()))
			}
		} else {
			sc.balance = bal
		}

		pos, err := sc.client.GetOpenPositions()
		if err != nil {
			pollHadError = true
			if strings.Contains(err.Error(), "unauthorized") || strings.Contains(err.Error(), "401") {
				m.notifier.Send("slave_auth_"+sc.config.AccountID, NotifyCritical,
					"Slave Auth Failed",
					fmt.Sprintf("Account #%s: %s", sc.config.AccountID, err.Error()))
			}
		} else {
			sc.positions = pos
		}

		// Update slave pending order list too
		if slavePending, err := sc.client.GetActiveOrders(); err != nil {
			pollHadError = true
		} else {
			sc.pendingKnown = pendingMap(slavePending)
		}

		// Recovery detection
		if hadErrorBefore && !pollHadError {
			m.notifier.Send("slave_recovered_"+sc.config.AccountID, NotifyWarning,
				"Slave Recovered",
				fmt.Sprintf("Account #%s connection restored", sc.config.AccountID))
		}
		sc.hadError = pollHadError
	}

	m.stats.mu.Lock()
	m.stats.masterPos = masterPos
	m.stats.masterPending = masterPending
	m.stats.masterBalance = masterBal
	m.stats.lastPoll = time.Now()
	m.stats.mu.Unlock()
}

// ────────────────────────────────────────────────────────────────────────────
// Views
// ────────────────────────────────────────────────────────────────────────────

// contentW returns the usable content width (terminal or a reasonable default).
func (m Model) contentW() int {
	if m.width > 20 {
		return m.width - 4
	}
	return 76 // fallback for unknown terminal
}

// truncate shortens s to fit within maxW terminal cells, appending "…" if cut.
func truncate(s string, maxW int) string {
	w := lipgloss.Width(s)
	if w <= maxW {
		return s
	}
	// Walk runes and build truncated string
	var out []rune
	runes := []rune(s)
	cur := 0
	for _, r := range runes {
		rw := lipgloss.Width(string(r))
		if cur+rw > maxW-1 {
			break
		}
		out = append(out, r)
		cur += rw
	}
	return string(out) + "…"
}

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
	case screenEdit:
		return m.viewEdit()
	case screenSettings:
		return m.viewSettings()
	}
	return ""
}

func (m Model) viewLogin() string {
	title := styleTitle.Render("  ◈  FundingPips Trade Copier  ")

	// Pre-fill notice
	notice := ""
	if cfg := loadConfig(); cfg != nil {
		slaveCount := len(cfg.Slaves)
		suffix := fmt.Sprintf("master #%s → %d slave(s)", cfg.MasterID, slaveCount)
		notice = styleDim.Render("  Saved config found — " + suffix)
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
	// Constrain to terminal width
	inner = lipgloss.NewStyle().MaxWidth(m.contentW()).Render(inner)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
}

func (m Model) viewAccounts() string {
	title := styleTitle.Render("  ◈  Select Accounts  ")

	var stepLabel string
	if m.pickStep == pickMaster {
		stepLabel = styleMasterTag.Render(" MASTER ") + styleDim.Render("  Choose the account to copy FROM")
	} else {
		// Show pending slaves count
		pendingStr := ""
		if len(m.pendingSlaves) > 0 {
			var ids []string
			for _, ps := range m.pendingSlaves {
				ids = append(ids, "#"+ps.AccountID+"×"+fmt.Sprintf("%.2f", ps.Multiplier))
			}
			pendingStr = "  " + styleDim.Render("| selected: ") + styleGreen.Render(strings.Join(ids, ", "))
		}
		stepLabel = styleSlaveTag.Render(" SLAVE ") + styleDim.Render("  Choose accounts to copy TO") +
			"  " + styleDim.Render("master: ") + styleBlue.Render("#"+m.masterID) + pendingStr
	}

	// If still verifying accounts (edit "Add slave"), show spinner
	if m.verifyingAccts {
		inner := lipgloss.JoinVertical(lipgloss.Left,
			title, "",
			stepLabel, "",
			"  "+m.spinner.View()+styleBlue.Render("  Verifying accounts..."),
		)
		inner = lipgloss.NewStyle().MaxWidth(m.contentW()).Render(inner)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
	}

	// Build a set of already-picked slave IDs
	pickedIDs := make(map[string]bool)
	for _, ps := range m.pendingSlaves {
		pickedIDs[ps.AccountID] = true
	}

	// Determine column widths from terminal width
	rowW := m.contentW() - 6  // panel border (4) + small padding
	idW := 10
	curW := 8
	typeW := 6
	nameW := rowW - idW - curW - typeW - 8 // 8 for "  " + "  " + " [" + "]" + extras
	if nameW < 10 {
		// Very narrow terminal — shrink ID and currency
		idW = 6
		curW = 4
		nameW = rowW - idW - curW - typeW - 8
		if nameW < 5 {
			nameW = 5
		}
	}

	// Build display rows — skip master and already-picked accounts during slave picking
	var rows []string
	displayIdx := 0 // visible index (skipping disabled items)
	doneIdx := -1
	for i, acc := range m.accounts {
		isMaster := acc.TradingAccountID == m.masterID
		isPicked := pickedIDs[acc.TradingAccountID]
		isDisabled := m.pickStep == pickSlave && (isMaster || isPicked)

		// In edit "Add slave" mode, skip accounts that failed verification (expired)
		if m.verifiedAccts != nil && m.pickStep == pickSlave && !isMaster && !isPicked {
			if !m.verifiedAccts[acc.TradingAccountID] {
				continue
			}
		}

		name := acc.Offer.Description
		if name == "" {
			name = acc.Offer.Name
		}
		acctType := "DEMO"
		if !acc.Offer.Demo {
			acctType = "LIVE"
		}

		if isMaster && m.pickStep == pickSlave {
			// Show master above the list as info, not in scrollable rows
			line := fmt.Sprintf("  ▶  #%-*s  %-*s  %-*s  [%s]",
				idW, truncate(acc.TradingAccountID, idW),
				curW, truncate(acc.Offer.Currency, curW),
				nameW, truncate(name, nameW),
				acctType)
			rows = append(rows, styleDim.Render(line)+"  "+styleMasterTag.Render(" SOURCE "))
			continue
		}

		if isDisabled {
			// Already-picked slaves — skip entirely during slave picking
			continue
		}

		line := fmt.Sprintf("  #%-*s  %-*s  %-*s  [%s]",
			idW, truncate(acc.TradingAccountID, idW),
			curW, truncate(acc.Offer.Currency, curW),
			nameW, truncate(name, nameW),
			acctType,
		)

		var row string
		if i == m.cursor {
			row = styleAccountRowFocused.Render(line)
		} else {
			row = styleAccountRow.Render(line)
		}
		rows = append(rows, row)
		displayIdx++
	}

	// Add "Done" option at the bottom in pickSlave mode (but not during edit)
	if m.pickStep == pickSlave && !m.editMode {
		doneIdx = len(m.accounts) // "Done" index = one past last account
		doneLabel := "  ✓  Done selecting slaves"
		if m.cursor == doneIdx {
			rows = append(rows, styleAccountRowFocused.Render(doneLabel))
		} else {
			rows = append(rows, styleAccountRow.Render(doneLabel))
		}
	}

	list := stylePanel.Render(strings.Join(rows, "\n"))

	help := styleDim.Render("  ↑↓ / j k  navigate    Enter / Space  select    Esc / b  back    q  quit")

	inner := lipgloss.JoinVertical(lipgloss.Left,
		title, "",
		stepLabel, "",
		list,
		"", help,
	)
	inner = lipgloss.NewStyle().MaxWidth(m.contentW()).Render(inner)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
}

func (m Model) viewMultiplier() string {
	title := styleTitle.Render("  ◈  Lot Multiplier  ")

	var summary string
	var btn string
	if m.slaveID != "" {
		// Configuring a specific slave
		summary = lipgloss.JoinHorizontal(lipgloss.Top,
			styleDim.Render("Master  "), styleMasterTag.Render(" #"+m.masterID+" "),
			styleDim.Render("  →  "),
			styleSlaveTag.Render(" #"+m.slaveID+" "), styleDim.Render("  Slave"),
		)
		btn = styleBtnFocused.Render(" ▶  Add Slave  ")
	} else {
		// Starting copier with all pending slaves
		var pendingStr string
		if len(m.pendingSlaves) > 0 {
			var ids []string
			for _, ps := range m.pendingSlaves {
				ids = append(ids, "#"+ps.AccountID+"×"+fmt.Sprintf("%.2f", ps.Multiplier))
			}
			pendingStr = styleGreen.Render(strings.Join(ids, ", "))
		}
		summary = lipgloss.JoinHorizontal(lipgloss.Top,
			styleDim.Render("Master  "), styleMasterTag.Render(" #"+m.masterID+" "),
			styleDim.Render("  →  "),
			styleSlaveTag.Render(fmt.Sprintf(" %d slave(s) ", len(m.pendingSlaves))),
			"  "+pendingStr,
		)
		btn = styleBtnFocused.Render(" ▶  Start Copier  ")
	}

	desc := styleDim.Render("  Scale slave lot size relative to master.\n  1.0 = same size  ·  0.5 = half  ·  2.0 = double")

	box := styleFocused.Render(m.multInput.View())

	errLine := ""
	if m.multErr != "" {
		errLine = "\n" + styleLogErr.Render("  ⚠  "+m.multErr)
	}

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
	inner = lipgloss.NewStyle().MaxWidth(m.contentW()).Render(inner)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
}

func (m Model) viewCopying() string {
	m.stats.mu.Lock()
	masterPos := m.stats.masterPos
	masterPending := m.stats.masterPending
	masterBal := m.stats.masterBalance
	lastPoll := m.stats.lastPoll
	m.stats.mu.Unlock()

	// Aggregate stats from all slaves
	totalCopied := 0
	totalClosed := 0
	totalPendCopied := 0
	totalPendCancelled := 0
	totalErrors := 0
	// Calculate responsive panel width based on terminal width
	nPanels := 1 + len(m.slaves)
	panelW := 52
	gap := 2
	if m.width > 0 {
		available := m.width - 4
		maxW := (available - (nPanels-1)*gap) / nPanels
		panelW = max(36, min(56, maxW))
	}

	var slavePanels []string
	for i, sc := range m.slaves {
		totalCopied += sc.copied
		totalClosed += sc.closed
		totalPendCopied += sc.pendingCopied
		totalPendCancelled += sc.pendingCancelled
		totalErrors += sc.errors
		label := fmt.Sprintf("SLAVE %d", i+1)
		slavePanels = append(slavePanels, m.renderAccount(label, sc.client, sc.balance, sc.positions, sc.pendingKnown, stylePanelSlave, panelW))
	}

	// Header — constrain to terminal width
	pollAge := ""
	if !lastPoll.IsZero() {
		ms := time.Since(lastPoll).Milliseconds()
		pollAge = styleDim.Render(fmt.Sprintf("  ⟳ %dms", ms))
	}
	statusBadge := styleBadgeGreen.Render(" LIVE ")
	statusText := pollAge
	if m.paused {
		statusBadge = styleBadgeAmber.Render("  ⏸ PAUSED  ")
		statusText = ""
	}
	titleW := m.contentW() - lipgloss.Width(statusBadge) - lipgloss.Width(statusText) - 4
	if titleW < 30 {
		titleW = 30
	}
	title := styleTitle.MaxWidth(titleW).Render("  ◈  FundingPips Trade Copier  ")
	header := lipgloss.JoinHorizontal(lipgloss.Center,
		title,
		"  ", statusBadge, statusText,
	)

	// Stats — compact for narrow terminals
	cpBadge := styleBadgeGreen.Render(fmt.Sprintf(" ✓ %d copied ", totalCopied))
	clBadge := styleBadgeAmber.Render(fmt.Sprintf(" ⊘ %d closed ", totalClosed))
	pcBadge := styleBadgeGreen.Render(fmt.Sprintf(" ⟳ %d pend copied ", totalPendCopied))
	pdBadge := styleBadgeAmber.Render(fmt.Sprintf(" ✕ %d pend cancelled ", totalPendCancelled))
	statsW := m.contentW()
	statsBar := lipgloss.JoinHorizontal(lipgloss.Top, cpBadge, "  ", clBadge, "  ", pcBadge, "  ", pdBadge)
	if totalErrors > 0 {
		errBadge := styleBadgeRed.Render(fmt.Sprintf(" ✗ %d errors ", totalErrors))
		statsBar = lipgloss.JoinHorizontal(lipgloss.Top, statsBar, "  ", errBadge)
	}
	// Truncate stats if still too wide
	if lipgloss.Width(statsBar) > statsW {
		statsBar = truncate(statsBar, statsW)
	}

	masterPanel := m.renderAccount("MASTER", m.master, masterBal, masterPos, pendingMap(masterPending), stylePanelMaster, panelW)

	// Stack vertically if terminal is too narrow, else lay out horizontally
	var panels string
	if m.width > 0 && m.width < nPanels*(panelW+gap)+4 {
		// Vertical stacking
		panels = masterPanel
		for _, sp := range slavePanels {
			panels += "\n\n" + sp
		}
	} else {
		// All panels side-by-side with gaps between
		all := make([]string, 0, nPanels*2-1)
		all = append(all, masterPanel)
		for _, sp := range slavePanels {
			all = append(all, strings.Repeat(" ", gap), sp)
		}
		panels = lipgloss.JoinHorizontal(lipgloss.Top, all...)
	}

	// Log
	logView := m.renderLog()

	// Footer with styled, clickable buttons
	var btnStrs []string
	if m.paused {
		btnStrs = append(btnStrs, styleFooterBtnAccent.Render(" ▶ p Resume "))
	} else {
		btnStrs = append(btnStrs, styleFooterBtn.Render(" ⏸ p Pause "))
	}
	btnStrs = append(btnStrs, styleFooterBtn.Render(" ⚙ s Settings "))
	btnStrs = append(btnStrs, styleFooterBtn.Render(" ✎ e Edit "))
	btnStrs = append(btnStrs, styleFooterBtn.Render(" ⇥ l Logout "))
	btnStrs = append(btnStrs, styleFooterBtn.Render(" ✕ q Quit "))
	footerBtns := lipgloss.JoinHorizontal(lipgloss.Top, btnStrs...)
	// Constrain footer width so it doesn't overflow
	footerBtns = lipgloss.NewStyle().MaxWidth(m.width).Render(footerBtns)

	body := lipgloss.JoinVertical(lipgloss.Left,
		header, "",
		statsBar, "",
		panels, "",
		logView, "",
		footerBtns,
	)
	return lipgloss.NewStyle().MaxWidth(m.width).Render(body)
}

func (m Model) viewSettings() string {
	title := styleTitle.Render("  ◈  Settings  ")

	line := fmt.Sprintf("  %s  %s",
		styleLabel.Render("Poll interval (ms):"),
		m.settingsInput.View(),
	)
	note := styleDim.Render("  Lower = faster copy, but may hit rate limits. Min 50ms.")
	footer := styleDim.Render("  enter  save    esc/b  back")

	inner := lipgloss.JoinVertical(lipgloss.Left,
		title, "",
		line, "",
		note, "",
		footer,
	)
	inner = lipgloss.NewStyle().MaxWidth(m.contentW()).Render(inner)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
}

func (m Model) viewEdit() string {
	title := styleTitle.Render("  ◈  Edit Configuration  ")

	masterLine := styleBlue.Render("⬟ MASTER") + "  " + styleLabel.Render("#"+m.masterID)

	var rows []string
	maxW := m.contentW() - 6
	rows = append(rows, "  "+truncate(masterLine, maxW), "")

	if len(m.pendingSlaves) == 0 {
		rows = append(rows, styleDim.Render("  — no slaves configured —"))
	} else {
		for i, ps := range m.pendingSlaves {
			line := fmt.Sprintf("  SLAVE %d:  #%-*s  ×%.2f", i+1,
				min(10, maxW-20), truncate(ps.AccountID, min(10, maxW-20)),
				ps.Multiplier)
			if i == m.cursor {
				rows = append(rows, styleAccountRowFocused.Render(line))
			} else {
				rows = append(rows, styleAccountRow.Render(line))
			}
		}
	}

	// "Add slave" option
	addLine := "  [+] Add slave"
	if m.cursor == len(m.pendingSlaves) {
		rows = append(rows, styleAccountRowFocused.Render(addLine))
	} else {
		rows = append(rows, styleAccountRow.Render(addLine))
	}

	// "Apply" option
	applyLine := "  [>] Apply & restart copier"
	if m.cursor == len(m.pendingSlaves)+1 {
		rows = append(rows, styleAccountRowFocused.Render(applyLine))
	} else {
		rows = append(rows, styleAccountRow.Render(applyLine))
	}

	list := stylePanel.Render(strings.Join(rows, "\n"))

	help := styleDim.Render("  ↑↓  navigate    r  remove slave    m  change multiplier    Enter  confirm    Esc  back    q  quit")

	inner := lipgloss.JoinVertical(lipgloss.Left,
		title, "",
		list,
		"", help,
	)
	inner = lipgloss.NewStyle().MaxWidth(m.contentW()).Render(inner)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
}

func (m Model) renderAccount(label string, c *Client, bal *BalanceResponse, positions []Position, pending map[string]PendingOrder, st lipgloss.Style, w int) string {
	// Inner content width = panel width minus border (2) and padding (2)
	innerW := w - 4
	if innerW < 24 {
		innerW = 24
	}

	var acctLabel string
	if label == "MASTER" {
		acctLabel = styleBlue.Render("⬟ MASTER") + "  " + styleLabel.Render("#"+c.accountID)
	} else {
		acctLabel = styleAmber.Render("⬟ SLAVE") + "   " + styleLabel.Render("#"+c.accountID)
	}

	// Truncate account name to fit
	nameStr := c.accountName
	if lipgloss.Width(nameStr) > innerW {
		nameStr = truncate(nameStr, innerW)
	}

	var lines []string
	lines = append(lines, acctLabel, styleDim.Render(nameStr), "")

	if bal != nil {
		pnl := bal.Profit
		pnlStyled := styleGreen.Render(pnl)
		if v, err := strconv.ParseFloat(pnl, 64); err == nil && v < 0 {
			pnlStyled = styleRed.Render(pnl)
		}
		lines = append(lines,
			truncate(styleLabel.Render("Balance  ")+styleBold.Render(bal.Balance+" "+bal.Currency), innerW),
			truncate(styleLabel.Render("Equity   ")+styleBold.Render(bal.Equity)+"   "+styleLabel.Render("P&L ")+pnlStyled, innerW),
			truncate(styleLabel.Render("Margin   ")+styleDim.Render(bal.FreeMargin+" free"), innerW),
			"",
		)
	}

	// Calculate column widths dynamically for position/pending tables
	// Available data area: 2 char prefix + columns + spaces between columns
	// Minimum viable column widths
	minSym := 4
	minSide := 3
	minVol := 5
	minPrice := 5
	minPnl := 5
	colGaps := 4 // spaces between 5 columns
	dataW := innerW - 2 // subtract "  " prefix

	// Distribute available width across columns
	symW, sideW, volW, priceW, pnlW := distCols(dataW, colGaps, minSym, minSide, minVol, minPrice, minPnl,
		// weight factors: symbol and P&L get more space
		2, 1, 1, 1, 2)

	// Pending orders use symbol + type+side + volume + price
	pendSymW, pendSSW, pendVolW, pendPriceW := pendColWidths(dataW, minSym, minSide, minVol, minPrice)

	if len(positions) == 0 {
		lines = append(lines, styleDim.Render(truncate("  — no open positions —", innerW)))
	} else {
		posHdr := fmt.Sprintf("  %-*s %-*s %-*s %-*s %-*s",
			symW, "Symbol", sideW, "Side", volW, "Vol", priceW, "Open@", pnlW, "P&L")
		lines = append(lines, styleDim.Render(truncate(posHdr, innerW)))
		for _, p := range positions {
			sideStr := styleGreen.Render(fmt.Sprintf("%-*s", sideW, p.Side))
			if p.Side == "SELL" {
				sideStr = styleRed.Render(fmt.Sprintf("%-*s", sideW, p.Side))
			}
			pnl := p.Profit
			pnlStr := styleGreen.Render(fmt.Sprintf("%-*s", pnlW, pnl))
			if v, err := strconv.ParseFloat(pnl, 64); err == nil && v < 0 {
				pnlStr = styleRed.Render(fmt.Sprintf("%-*s", pnlW, pnl))
			}
			row := fmt.Sprintf("  %-*s %s %-*s %-*s %s",
				symW, truncate(p.Symbol, symW),
				sideStr, volW, p.Volume,
				priceW, truncate(p.OpenPrice, priceW),
				pnlStr,
			)
			lines = append(lines, truncate(row, innerW))
		}
	}

	// Pending orders section
	if len(pending) > 0 {
		pendHdr := fmt.Sprintf("  %-*s %-*s %-*s %-*s",
			pendSymW, "Symbol", pendSSW, "Side", pendVolW, "Vol", pendPriceW, "Trigger@")
		lines = append(lines, "",
			truncate(styleDim.Render(fmt.Sprintf("  ⏳ %d pending", len(pending))), innerW),
			styleDim.Render(truncate(pendHdr, innerW)))
		for _, po := range pending {
			sideStr := styleGreen.Render(fmt.Sprintf("%-*s", minSide, po.Side))
			if po.Side == "SELL" {
				sideStr = styleRed.Render(fmt.Sprintf("%-*s", minSide, po.Side))
			}
			typeStr := styleAmber.Render(fmt.Sprintf("%-*s", minSide, po.Type))
			row := fmt.Sprintf("  %-*s %s%s %-*s %-*s",
				pendSymW, truncate(po.Symbol, pendSymW),
				typeStr, sideStr,
				pendVolW, po.Volume,
				pendPriceW, truncate(po.ActivationPrice, pendPriceW),
			)
			lines = append(lines, truncate(row, innerW))
		}
	}

	return st.MaxWidth(w).Render(strings.Join(lines, "\n"))
}

// distCols distributes available width across 5 columns with given min widths and weights.
func distCols(avail, gaps, min1, min2, min3, min4, min5 int, w1, w2, w3, w4, w5 int) (int, int, int, int, int) {
	mins := min1 + min2 + min3 + min4 + min5 + gaps
	if avail <= mins {
		return min1, min2, min3, min4, min5
	}
	extra := avail - mins
	totalW := w1 + w2 + w3 + w4 + w5
	return min1 + extra*w1/totalW,
		min2 + extra*w2/totalW,
		min3 + extra*w3/totalW,
		min4 + extra*w4/totalW,
		min5 + extra*w5/totalW
}

// pendColWidths distributes available width across 4 pending-order columns.
func pendColWidths(avail, minSym, minSide, minVol, minPrice int) (int, int, int, int) {
	// Type+Side combined column needs at least minSide*2
	ssMin := minSide * 2
	gaps := 3
	mins := minSym + ssMin + minVol + minPrice + gaps
	if avail <= mins {
		return minSym, ssMin, minVol, minPrice
	}
	extra := avail - mins
	return minSym + extra*2/7,
		ssMin + extra/7,
		minVol + extra*2/7,
		minPrice + extra*2/7
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
	p := tea.NewProgram(newModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
