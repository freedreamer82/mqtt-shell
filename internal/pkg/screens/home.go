package screens

import (
	"fmt"
	"image/color"
	"io"
	"strings"

	"github.com/freedreamer82/mqtt-shell/pkg/info"
	mqtt "github.com/freedreamer82/mqtt-shell/pkg/mqttchat"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/config"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/constant"
)

type blackRenderer struct {
	fyne.WidgetRenderer
}

func (p *blackRenderer) BackgroundColor() color.Color {
	return color.RGBA{255, 20, 147, 255}
}

type MainScreen struct {
	container     fyne.CanvasObject
	sendButton    *widget.Button
	clearButton   *widget.Button
	input         *widget.Entry
	isConnected   bool
	cmdScreen     *CmdScreen
	clientName    *widget.Entry
	shell         *widget.Label
	mqttScreen    *MqttDialog
	connectedText *widget.Label
	verText       *widget.Label
	client        *mqtt.MqttClientChat
	app           fyne.App
	appWindow     fyne.Window
	scroll        *container.Scroll
	chanReadReady chan bool
	connectedIcon *widget.Icon
	waitBar       *WaitBar
	inputcmd      string
	inputText     string
}

func (s *MainScreen) Read(p []byte) (n int, err error) {

	select {
	case r := <-s.chanReadReady:
		if !r {
			return 0, nil
		}
	}

	if s.input != nil {
		data := s.input.Text + "\n"
		if len(data) != 0 {
			n = copy(p, data)
			fyne.DoAndWait(func() {
				s.input.SetText("")
			})
			return len(data), nil
		} else if len(p) == 0 {
			// If the caller wanted a zero byte read, return immediately
			// without trying (but after acquiring the readLock).
			// Otherwise syscall.Read returns 0, nil which looks like
			// io.EOF.
			return 0, nil
		}
	}
	return 0, nil
}

const shellHistoryDepthLines = 100

func (s *MainScreen) Write(p []byte) (n int, err error) {

	if s.waitBar != nil && s.waitBar.Visible() {
		s.waitBar.Hide()
	}

	toTrim := string(p)
	trimmed := strings.Replace(toTrim, "\r\n", "\n", -1)

	text := s.shell.Text + trimmed
	lines := strings.Split(text, "\n")

	startIdx := len(lines) - shellHistoryDepthLines
	if startIdx < 0 {
		startIdx = 0
	}

	text = ""
	for i := startIdx; i < len(lines); i++ {
		text += lines[i]
		if i < len(lines)-1 {
			text += "\n"
		}
	}

	s.inputText = text

	fyne.DoAndWait(func() {
		s.shell.SetText(text)
		s.scroll.ScrollToBottom()
	})

	return len(p), nil
}

func (s *MainScreen) createRenderer() fyne.WidgetRenderer {
	r := s.shell.CreateRenderer()
	return &blackRenderer{r}
}

func (s *MainScreen) clientCb(c string) {

	s.clientName.SetText(c)

	s.connectedText.SetText(constant.HOME_SCREEN_Broker_Connected)
	s.verText.SetText(info.VERSION)
	s.connectedIcon.SetResource(theme.ConfirmIcon())

	txTopic := fmt.Sprintf(config.TemplateSubTopic, c)
	rxTopic := fmt.Sprintf(config.TemplateSubTopicreply, c)

	if s.client != nil && s.client.IsRunning() {
		s.client.Stop()
		s.client = nil
	}

	if s.client == nil {
		c := struct {
			io.Reader
			io.Writer
		}{s, s}

		s.client = mqtt.NewClientChatWithCustomIO(s.mqttScreen.mqttOpts, rxTopic, txTopic, info.VERSION, c)
	}

	s.client.Start()

	if s.waitBar != nil && !s.waitBar.Visible() {
		s.waitBar.Resize(fyne.NewSize(s.appWindow.Canvas().Size().Width/2, s.appWindow.Canvas().Size().Height/20))
		s.waitBar.Show()
	}

}

func (s *MainScreen) onCloseCmdCb(screen *CmdScreen, cmd string) {
	if cmd != "" {
		s.input.SetText(cmd)

	}
}

func (s *MainScreen) GetContainer() fyne.CanvasObject {
	return s.container
}

func (s *MainScreen) clear() {
	if s.shell != nil {
		s.shell.SetText("")
		s.inputText = ""
		s.input.SetText("")
	}
}

func NewMainScreen(app fyne.App, appWindow fyne.Window) *MainScreen {

	input := widget.NewEntry()

	s := MainScreen{mqttScreen: NewMqttDialog(appWindow, app.Preferences()),
		cmdScreen: NewCmdOverlay(appWindow, app.Preferences()),
		input:     input, app: app, appWindow: appWindow}
	s.chanReadReady = make(chan bool)
	s.cmdScreen.SetOnCloseCallback(s.onCloseCmdCb)

	clearButton := widget.NewButton(constant.HOME_SCREEN_ClearButton, func() {
		s.clear()
	})

	sendButton := widget.NewButton(constant.HOME_SCREEN_SendButton, func() {
		s.Write([]byte("\n"))
		s.chanReadReady <- true
	})

	input.OnSubmitted = func(tosend string) {
		//s.Write([]byte(fmt.Sprintf("%s", tosend)))
		s.Write([]byte("\n"))
		s.chanReadReady <- true
	}

	input.OnChanged = func(cmd string) {
		s.inputcmd = cmd
		s.shell.SetText(s.inputText + s.inputcmd)
		s.shell.Refresh()
	}

	s.shell = widget.NewLabel("")
	r := s.createRenderer()
	r.Refresh()
	//s.shell.Disable()

	s.clientName = widget.NewEntry()
	s.clientName.Disable()

	s.waitBar = NewWaitBar(s.appWindow)

	scan := widget.NewButton("", func() {

		s.mqttScreen.scanScreen.Scan()
	})

	scan.SetIcon(theme.SearchIcon())
	icon := theme.MediaRecordIcon()

	s.connectedText = widget.NewLabel(constant.HOME_SCREEN_Broker_Disconnected)
	s.verText = widget.NewLabel(info.VERSION)

	s.connectedIcon = widget.NewIcon(icon)
	s.connectedIcon.SetResource(theme.ContentClearIcon())

	addCommandButton := widget.NewButton("", func() {
		s.cmdScreen.AddNewCommand(input.Text)
	})
	addCommandButton.SetIcon(theme.ContentAddIcon())

	cmdListButton := widget.NewButton("", func() {
		s.cmdScreen.ShowPopUp()

	})
	cmdListButton.SetIcon(theme.MenuDropUpIcon())

	clearInput := widget.NewButton("", func() {
		s.input.SetText("")
	})
	clearInput.SetIcon(theme.ContentClearIcon())

	s.scroll = container.NewScroll(s.shell)

	cont := container.NewBorder(
		container.NewBorder(nil, nil, nil, container.NewHBox(scan, s.connectedText, s.connectedIcon, s.verText), s.clientName),
		container.NewBorder(nil, nil, nil, container.NewHBox(clearInput, addCommandButton, cmdListButton, clearButton, sendButton), input),
		nil, nil,
		s.scroll)

	s.container = cont
	s.input = input
	s.sendButton = sendButton
	s.sendButton = clearButton

	s.mqttScreen.scanScreen.SetCallbackClient(s.clientCb)

	//fyne.CurrentDevice().IsMobile()

	return &s
}
