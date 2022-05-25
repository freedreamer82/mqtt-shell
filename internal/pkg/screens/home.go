package screens

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/config"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/constant"
	mqtt "github.com/freedreamer82/mqtt-shell/internal/pkg/mqtt2shell"
	"image/color"
	"io"
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
	input         *widget.Entry
	isConnected   bool
	cmdScreen     *CmdScreen
	clientName    *widget.Entry
	shell         *widget.TextGrid
	mqttScreen    *MqttDialog
	connectedText *widget.Label
	client        *mqtt.MqttClientChat
	app           fyne.App
	scroll        *container.Scroll
	chanReadReady chan bool
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
			s.input.SetText("")
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

func (s *MainScreen) Write(p []byte) (n int, err error) {
	//trimmed := strings.Replace(string(p), "\r\n", "\n", -1)
	trimmed := string(p)
	//i := strings.LastIndex(trimmed, "\r\n")
	//if i > 0 {
	//	trimmed = trimmed[:i]
	//}
	//excludingLast := trimmed[:i] + strings.Replace(trimmed[i:], "\n", "", 1)
	//fmt.Println(excludingLast)

	s.shell.SetText(s.shell.Text() + trimmed)
	s.scroll.ScrollToBottom()
	s.scroll.Refresh()

	return len(p), nil
}

func (s *MainScreen) createRenderer() fyne.WidgetRenderer {
	r := s.shell.CreateRenderer()
	return &blackRenderer{r}
}

func (s *MainScreen) clientCb(c string) {
	s.clientName.SetText(c)

	s.connectedText.SetText("connected")

	txTopic := fmt.Sprintf(config.TemplateSubTopicreply, c)
	rxTopic := fmt.Sprintf(config.TemplateSubTopic, c)

	if s.client == nil {
		c := struct {
			io.Reader
			io.Writer
		}{s, s}

		s.client = mqtt.NewClientChatWithCustomIO(s.mqttScreen.mqttOpts, txTopic, rxTopic, constant.VERSION, c)
	}

	if s.client.IsRunning() {
		s.client.Stop()
	}
	s.client.Start()

}

//const example = "-------------------------------------------------\n|  Mqtt-shell client \n|\n|  IP: 192.168.50.154 \n|  SERVER VER: 0.0.5 - CLIENT VER: 0.0.5\n|  TX: /mqtt-shell/mouseberry-polimi/cmd/res\n|  RX: /mqtt-shell/mouseberry-polimi/cmd/res\n|\n-------------------------------------------------\n>\n-------------------------------------------------\n|  Mqtt-shell client \n|\n|  IP: 192.168.50.154 \n|  SERVER VER: 0.0.5 - CLIENT VER: 0.0.5\n|  TX: /mqtt-shell/mouseberry-polimi/cmd/res\n|  RX: /mqtt-shell/mouseberry-polimi/cmd/res\n|\n-------------------------------------------------\n>\n-------------------------------------------------\n|  Mqtt-shell client \n|\n|  IP: 192.168.50.154 \n|  SERVER VER: 0.0.5 - CLIENT VER: 0.0.5\n|  TX: /mqtt-shell/mouseberry-polimi/cmd/res\n|  RX: /mqtt-shell/mouseberry-polimi/cmd/res\n|\n-------------------------------------------------\n>\n"

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
	}
}

func NewMainScreen(app fyne.App, appWindow fyne.Window) *MainScreen {
	input := widget.NewEntry()

	s := MainScreen{mqttScreen: NewMqttDialog(appWindow, app.Preferences()), cmdScreen: NewCmdOverlay(appWindow), input: input, app: app}
	s.chanReadReady = make(chan bool)
	s.cmdScreen.SetOnCloseCallback(s.onCloseCmdCb)

	sendButton := widget.NewButton("clear", func() {
		s.clear()
	})
	input.OnSubmitted = func(tosend string) {
		s.chanReadReady <- true
	}

	s.shell = widget.NewTextGrid()
	r := s.createRenderer()
	r.Refresh()
	//s.shell.Disable()

	s.clientName = widget.NewEntry()
	s.clientName.Disable()

	scan := widget.NewButton("", func() {

		s.mqttScreen.scanScreen.Scan()
	})

	scan.SetIcon(theme.SearchIcon())
	icon := theme.MediaRecordIcon()

	s.connectedText = widget.NewLabel("disconnected")
	connectedIcon := widget.NewIcon(icon)
	connectedIcon.SetResource(theme.ContentClearIcon())

	addCommandButton := widget.NewButton("", func() {
		s.cmdScreen.AddNewCommand(input.Text)
	})
	addCommandButton.SetIcon(theme.ContentAddIcon())

	cmdListButton := widget.NewButton("", func() {
		s.cmdScreen.ShowPopUp()

	})
	cmdListButton.SetIcon(theme.MenuDropUpIcon())

	s.scroll = container.NewVScroll(s.shell)

	cont := container.NewBorder(container.NewBorder(nil, nil, nil,
		container.NewHBox(scan, s.connectedText, connectedIcon), s.clientName),
		container.NewBorder(nil, nil, nil, container.NewHBox(addCommandButton, cmdListButton, sendButton), input), nil, nil,
		s.scroll)

	cont.Resize(fyne.NewSize(300, 300))

	s.container = cont
	s.input = input
	s.sendButton = sendButton

	s.mqttScreen.scanScreen.SetCallbackClient(s.clientCb)

	//s.shell.SetText(example)

	return &s
}
