package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
	gkclient "github.com/zhebrikov/gophkeeper/internal/client"
)

type screen int

const (
	screenAuth screen = iota
	screenMaster
	screenList
	screenDetail
	screenForm
	screenConfirm
)

const (
	formAdd  = "add"
	formEdit = "edit"
)

type secretItem struct {
	secret *pb.SecretResponse
}

func (i secretItem) Title() string       { return i.secret.GetName() }
func (i secretItem) Description() string { return secretTypeName(i.secret.GetType()) }
func (i secretItem) FilterValue() string { return i.secret.GetName() }

type model struct {
	ctx context.Context
	api *gkclient.API

	screen         screen
	width, height  int
	errMsg         string
	statusMsg      string
	loading        bool
	masterPassword string

	authRegister bool
	authInputs   []textinput.Model
	authFocus    int

	masterInput textinput.Model

	secretList list.Model
	secrets    []*pb.SecretResponse

	detailSecret    *pb.SecretResponse
	detailPlaintext string

	formMode       string
	formInputs     []textinput.Model
	formFocus      int
	editingID      string
	editingVersion int64
	editingType    pb.SecretType

	confirmID     string
	editAfterLoad bool
}

func newModel(ctx context.Context, api *gkclient.API) model {
	authInputs := newAuthInputs()
	masterInput := newPasswordInput("Master password")
	formInputs := newFormInputs()

	delegate := list.NewDefaultDelegate()
	secretList := list.New([]list.Item{}, delegate, 0, 0)
	secretList.Title = secretListTitle()
	secretList.SetShowStatusBar(true)
	secretList.SetFilteringEnabled(true)
	secretList.Styles.Title = titleStyle
	secretList.Styles.PaginationStyle = subtitleStyle
	secretList.Styles.HelpStyle = helpStyle

	m := model{
		ctx:         ctx,
		api:         api,
		authInputs:  authInputs,
		masterInput: masterInput,
		formInputs:  formInputs,
		secretList:  secretList,
	}

	if api.IsAuthenticated() {
		m.screen = screenMaster
		m.masterInput.Focus()
	} else {
		m.screen = screenAuth
		m.authInputs[0].Focus()
	}

	return m
}

func newAuthInputs() []textinput.Model {
	login := textinput.New()
	login.Placeholder = "Login"
	login.CharLimit = 128
	login.Width = 40

	password := newPasswordInput("Password")
	return []textinput.Model{login, password}
}

func newPasswordInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.CharLimit = 256
	ti.Width = 40
	return ti
}

func newFormInputs() []textinput.Model {
	labels := []string{"Type (credential/text/binary/card/otp)", "Name", "Data", "Metadata (optional)"}
	inputs := make([]textinput.Model, len(labels))
	for i, label := range labels {
		inputs[i] = textinput.New()
		inputs[i].Placeholder = label
		inputs[i].CharLimit = 4096
		inputs[i].Width = 50
	}
	return inputs
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.secretList.SetWidth(msg.Width)
		m.secretList.SetHeight(msg.Height - 6)
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}
		switch m.screen {
		case screenAuth:
			return m.updateAuth(msg)
		case screenMaster:
			return m.updateMaster(msg)
		case screenList:
			return m.updateList(msg)
		case screenDetail:
			return m.updateDetail(msg)
		case screenForm:
			return m.updateForm(msg)
		case screenConfirm:
			return m.updateConfirm(msg)
		}

	case authDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = errString(msg.err)
			return m, nil
		}
		m.errMsg = ""
		if msg.register {
			m.statusMsg = "Registration successful"
		} else {
			m.statusMsg = "Login successful"
		}
		m.screen = screenMaster
		m.masterInput.Reset()
		m.masterInput.Focus()
		return m, textinput.Blink

	case secretsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = errString(msg.err)
			return m, nil
		}
		m.secrets = msg.secrets
		m.errMsg = ""
		m.secretList.SetItems(secretsToItems(msg.secrets))
		m.screen = screenList
		return m, nil

	case secretDetailMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = errString(msg.err)
			m.editAfterLoad = false
			m.screen = screenList
			return m, nil
		}
		plaintext := formatSecretData(msg.secret.GetType(), msg.plaintext)
		if m.editAfterLoad {
			m.editAfterLoad = false
			m.openForm(formEdit, msg.secret, plaintext)
			return m, textinput.Blink
		}
		m.detailSecret = msg.secret
		m.detailPlaintext = plaintext
		m.screen = screenDetail
		return m, nil

	case actionDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = errString(msg.err)
			return m, nil
		}
		m.statusMsg = msg.message
		m.errMsg = ""
		if msg.message == "Logged out" {
			m.masterPassword = ""
			m.secrets = nil
			m.secretList.SetItems(nil)
			m.screen = screenAuth
			m.authRegister = false
			for i := range m.authInputs {
				m.authInputs[i].Reset()
			}
			m.authFocus = 0
			m.authInputs[0].Focus()
			return m, textinput.Blink
		}
		if strings.HasPrefix(msg.message, "Synced") || msg.message == "Secret deleted" ||
			m.screen == screenConfirm || m.screen == screenForm {
			m.loading = true
			return m, loadSecretsCmd(m.ctx, m.api)
		}
		return m, nil
	}

	var cmd tea.Cmd
	switch m.screen {
	case screenAuth:
		m.authInputs[m.authFocus], cmd = m.authInputs[m.authFocus].Update(msg)
	case screenMaster:
		m.masterInput, cmd = m.masterInput.Update(msg)
	case screenList:
		m.secretList, cmd = m.secretList.Update(msg)
	case screenForm:
		m.formInputs[m.formFocus], cmd = m.formInputs[m.formFocus].Update(msg)
	}

	return m, cmd
}

func (m model) View() string {
	var b strings.Builder

	switch m.screen {
	case screenAuth:
		b.WriteString(m.viewAuth())
	case screenMaster:
		b.WriteString(m.viewMaster())
	case screenList:
		b.WriteString(m.viewList())
	case screenDetail:
		b.WriteString(m.viewDetail())
	case screenForm:
		b.WriteString(m.viewForm())
	case screenConfirm:
		b.WriteString(m.viewConfirm())
	}

	if m.loading {
		b.WriteString("\n" + subtitleStyle.Render("Loading..."))
	}
	if m.errMsg != "" {
		b.WriteString("\n" + errorStyle.Render("Error: "+m.errMsg))
	}
	if m.statusMsg != "" && m.errMsg == "" {
		b.WriteString("\n" + statusStyle.Render(m.statusMsg))
	}

	return b.String()
}

func (m model) viewAuth() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("GophKeeper") + "\n\n")

	loginTab := inactiveTabStyle.Render("Login")
	registerTab := inactiveTabStyle.Render("Register")
	if m.authRegister {
		registerTab = activeTabStyle.Render("Register")
	} else {
		loginTab = activeTabStyle.Render("Login")
	}
	b.WriteString(loginTab + "   " + registerTab + "\n\n")

	labels := []string{"Login", "Password"}
	for i, input := range m.authInputs {
		prefix := "  "
		if i == m.authFocus {
			prefix = labelStyle.Render("> ")
		}
		b.WriteString(prefix + labels[i] + ": " + input.View() + "\n")
	}

	b.WriteString(footerHelp("tab: switch mode", "enter: submit", "ctrl+c: quit"))
	return b.String()
}

func (m model) viewMaster() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("GophKeeper") + "\n\n")
	b.WriteString(subtitleStyle.Render("Enter master password to decrypt secrets") + "\n\n")
	b.WriteString(labelStyle.Render("Master password") + ": " + m.masterInput.View() + "\n")
	b.WriteString(footerHelp("enter: continue", "ctrl+c: quit"))
	return b.String()
}

func (m model) viewList() string {
	return m.secretList.View() + "\n" + footerHelp(
		"enter: view",
		"n: new",
		"e: edit",
		"d: delete",
		"s: sync",
		"l: logout",
		"r: refresh",
		"q: quit",
	)
}

func (m model) viewDetail() string {
	if m.detailSecret == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.detailSecret.GetName()) + "\n\n")
	b.WriteString(detailLine("ID", m.detailSecret.GetId()) + "\n")
	b.WriteString(detailLine("Type", secretTypeName(m.detailSecret.GetType())) + "\n")
	b.WriteString(detailLine("Metadata", m.detailSecret.GetMetadata()) + "\n")
	b.WriteString(detailLine("Version", fmt.Sprintf("%d", m.detailSecret.GetVersion())) + "\n")
	b.WriteString(detailLine("Updated", formatUnix(m.detailSecret.GetUpdatedAt())) + "\n")
	b.WriteString(detailLine("Data", truncate(m.detailPlaintext, 200)) + "\n")
	b.WriteString(footerHelp("esc: back", "e: edit", "d: delete"))
	return b.String()
}

func (m model) viewForm() string {
	var b strings.Builder
	title := "New secret"
	if m.formMode == formEdit {
		title = "Edit secret"
	}
	b.WriteString(titleStyle.Render(title) + "\n\n")

	labels := []string{"Type", "Name", "Data", "Metadata"}
	for i, input := range m.formInputs {
		prefix := "  "
		if i == m.formFocus {
			prefix = labelStyle.Render("> ")
		}
		b.WriteString(prefix + labels[i] + ": " + input.View() + "\n")
	}

	b.WriteString(footerHelp("tab/shift+tab: fields", "enter: save", "esc: cancel"))
	return b.String()
}

func (m model) viewConfirm() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Confirm delete") + "\n\n")
	b.WriteString("Delete secret " + m.confirmID + "?\n")
	b.WriteString(footerHelp("y: confirm", "n/esc: cancel"))
	return b.String()
}

func (m model) updateAuth(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "tab":
		m.authRegister = !m.authRegister
		m.errMsg = ""
		return m, nil
	case "shift+tab":
		m.authRegister = !m.authRegister
		m.errMsg = ""
		return m, nil
	case "up", "down":
		m.authFocus = 1 - m.authFocus
		for i := range m.authInputs {
			if i == m.authFocus {
				m.authInputs[i].Focus()
			} else {
				m.authInputs[i].Blur()
			}
		}
		return m, textinput.Blink
	case "enter":
		login := strings.TrimSpace(m.authInputs[0].Value())
		password := m.authInputs[1].Value()
		if login == "" || password == "" {
			m.errMsg = "login and password are required"
			return m, nil
		}
		m.loading = true
		m.errMsg = ""
		return m, authCmd(m.ctx, m.api, m.authRegister, login, password)
	}

	var cmd tea.Cmd
	m.authInputs[m.authFocus], cmd = m.authInputs[m.authFocus].Update(msg)
	return m, cmd
}

func (m model) updateMaster(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "enter":
		master := m.masterInput.Value()
		if master == "" {
			m.errMsg = "master password is required"
			return m, nil
		}
		m.masterPassword = master
		m.loading = true
		m.errMsg = ""
		return m, loadSecretsCmd(m.ctx, m.api)
	}

	var cmd tea.Cmd
	m.masterInput, cmd = m.masterInput.Update(msg)
	return m, cmd
}

func (m model) updateList(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "n":
		m.openForm(formAdd, nil, "")
		return m, textinput.Blink
	case "r":
		m.loading = true
		m.statusMsg = ""
		return m, loadSecretsCmd(m.ctx, m.api)
	case "s":
		m.loading = true
		m.statusMsg = ""
		return m, syncCmd(m.ctx, m.api)
	case "l":
		m.loading = true
		m.statusMsg = ""
		return m, logoutCmd(m.api)
	case "d":
		if item, ok := m.secretList.SelectedItem().(secretItem); ok {
			m.confirmID = item.secret.GetId()
			m.screen = screenConfirm
		}
		return m, nil
	case "e":
		if item, ok := m.secretList.SelectedItem().(secretItem); ok {
			m.editAfterLoad = true
			m.loading = true
			return m, getSecretCmd(m.ctx, m.api, item.secret.GetId(), m.masterPassword)
		}
		return m, nil
	case "enter":
		if item, ok := m.secretList.SelectedItem().(secretItem); ok {
			m.loading = true
			return m, getSecretCmd(m.ctx, m.api, item.secret.GetId(), m.masterPassword)
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.secretList, cmd = m.secretList.Update(msg)
	return m, cmd
}

func (m model) updateDetail(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		m.screen = screenList
		return m, nil
	case "e":
		m.openForm(formEdit, m.detailSecret, m.detailPlaintext)
		return m, textinput.Blink
	case "d":
		if m.detailSecret != nil {
			m.confirmID = m.detailSecret.GetId()
			m.screen = screenConfirm
		}
		return m, nil
	}
	return m, nil
}

func (m model) updateForm(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenList
		return m, nil
	case "tab", "down":
		m.formFocus = (m.formFocus + 1) % len(m.formInputs)
		m.focusFormField()
		return m, textinput.Blink
	case "shift+tab", "up":
		m.formFocus = (m.formFocus - 1 + len(m.formInputs)) % len(m.formInputs)
		m.focusFormField()
		return m, textinput.Blink
	case "enter":
		return m.submitForm()
	}

	var cmd tea.Cmd
	m.formInputs[m.formFocus], cmd = m.formInputs[m.formFocus].Update(msg)
	return m, cmd
}

func (m model) updateConfirm(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		m.loading = true
		m.screen = screenList
		return m, deleteSecretCmd(m.ctx, m.api, m.confirmID)
	case "n", "esc":
		m.screen = screenList
		return m, nil
	}
	return m, nil
}

func (m *model) focusFormField() {
	for i := range m.formInputs {
		if i == m.formFocus {
			m.formInputs[i].Focus()
		} else {
			m.formInputs[i].Blur()
		}
	}
}

func (m *model) openForm(mode string, secret *pb.SecretResponse, plaintext string) {
	m.formMode = mode
	m.formFocus = 0
	m.editingID = ""
	m.editingVersion = 0
	m.editingType = pb.SecretType_SECRET_TYPE_TEXT

	for i := range m.formInputs {
		m.formInputs[i].Reset()
	}

	if mode == formEdit && secret != nil {
		m.editingID = secret.GetId()
		m.editingVersion = secret.GetVersion()
		m.editingType = secret.GetType()
		m.formInputs[0].SetValue(secretTypeName(secret.GetType()))
		m.formInputs[1].SetValue(secret.GetName())
		m.formInputs[2].SetValue(plaintext)
		m.formInputs[3].SetValue(secret.GetMetadata())
	}

	m.focusFormField()
	m.screen = screenForm
	m.errMsg = ""
}

func (m model) submitForm() (model, tea.Cmd) {
	typ := parseSecretType(m.formInputs[0].Value())
	name := strings.TrimSpace(m.formInputs[1].Value())
	data := m.formInputs[2].Value()
	metadata := m.formInputs[3].Value()

	if name == "" || data == "" {
		m.errMsg = "name and data are required"
		return m, nil
	}

	m.loading = true
	m.errMsg = ""

	if m.formMode == formAdd {
		return m, createSecretCmd(m.ctx, m.api, m.masterPassword, typ, name, data, metadata)
	}

	if m.editingID == "" {
		m.errMsg = "missing secret id"
		m.loading = false
		return m, nil
	}

	return m, updateSecretCmd(m.ctx, m.api, m.masterPassword, m.editingID, name, data, metadata, m.editingVersion)
}

func secretsToItems(secrets []*pb.SecretResponse) []list.Item {
	items := make([]list.Item, len(secrets))
	for i, s := range secrets {
		items[i] = secretItem{secret: s}
	}
	return items
}
