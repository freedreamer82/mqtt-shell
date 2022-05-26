package screens

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type CmdScreen struct {
	container    fyne.CanvasObject
	cmds         []string
	selectedCmd  int
	inputcmd     binding.String
	listData     *widget.List
	app          fyne.Window
	cancelButton *widget.Button
	okButton     *widget.Button
	onCLoseCb    OnClosePopUp
	storage      fyne.Preferences
	cmdNo        int
}

type OnClosePopUp func(screen *CmdScreen, cmd string)

func (s *CmdScreen) SetOnCloseCallback(cb OnClosePopUp) {
	s.onCLoseCb = cb
}

func (s *CmdScreen) GetContainer() fyne.CanvasObject {
	return s.container
}

func (s *CmdScreen) AddNewCommand(cmd string) error {

	s.addCommandAndShowInfo(cmd)
	return nil
}

func (s *CmdScreen) RemoveSelectedCommand() error {
	s.cmds = append(s.cmds[:s.selectedCmd], s.cmds[s.selectedCmd+1:]...)
	return nil
}

func (s *CmdScreen) GetCommands() []string {
	return s.cmds
}

func (s *CmdScreen) addCommandAndShowInfo(cmd string) {

	if cmd != "" {
		c := fmt.Sprintf("%s", cmd)
		dialog.ShowInformation("Cmd Added", c, s.app)
		s.cmds = append(s.cmds, cmd)
		s.storage.SetString(keyCmd+strconv.Itoa(s.cmdNo), cmd)
		s.cmdNo++
		s.storage.SetInt(keyCmdNumber, s.cmdNo)
	}
}
func (s *CmdScreen) notifyCb() {
	if s.onCLoseCb != nil {
		cmd := ""
		if s.selectedCmd >= 0 {
			cmd = s.cmds[s.selectedCmd]
		}
		s.onCLoseCb(s, cmd)
	}
}

func (s *CmdScreen) ShowPopUp() {
	s.selectedCmd = -1

	s.cancelButton = widget.NewButton("Cancel", func() {
		s.container.Hide()
		s.notifyCb()

	})

	s.okButton = widget.NewButton("Ok", func() {
		s.container.Hide()
		s.notifyCb()
	})
	s.okButton.Disable()

	var cmdDeleteButton = widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {

		if s.listData != nil && s.selectedCmd >= 0 {
			s.cmds = append(s.cmds[:s.selectedCmd], s.cmds[s.selectedCmd+1:]...)
			s.storage.RemoveValue(keyCmd + strconv.Itoa(s.selectedCmd))
			s.cmdNo--
			s.storage.SetInt(keyCmdNumber, s.cmdNo)
		}

		if len(s.cmds) == 0 {
			s.okButton.Disable()
		}
		s.selectedCmd = -1
		s.listData.Refresh()

	})

	s.listData = widget.NewList(
		func() int {
			return len(s.cmds)
		},
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil, nil, container.NewHBox(layout.NewSpacer(), widget.NewSeparator(), cmdDeleteButton), widget.NewLabel("empty"))
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*fyne.Container).Objects[0].(*widget.Label).SetText(s.cmds[id])
		},
	)

	//s.listData.OnUnselected = func(id widget.ListItemID) {
	//	fmt.Println("unselected")
	//}
	s.listData.OnSelected = func(id widget.ListItemID) {
		//fmt.Printf("selected %d", id)
		//popup.Hide()
		s.okButton.Enable()
		s.selectedCmd = id
	}

	c := container.NewBorder(nil, container.NewHBox(s.cancelButton, s.okButton), nil, nil,
		container.NewScroll(s.listData))

	s.container = widget.NewModalPopUp(c, s.app.Canvas())

	s.container.Resize(fyne.NewSize(s.app.Canvas().Size().Width/2, s.app.Canvas().Size().Height/2))
	s.container.Show()
}

const keyCmdNumber = "cmdnumber"
const keyCmd = "cmd"

func NewCmdOverlay(app fyne.Window, storage fyne.Preferences) *CmdScreen {

	s := CmdScreen{onCLoseCb: nil}
	s.selectedCmd = -1
	s.storage = storage
	s.app = app

	s.cmdNo = s.storage.IntWithFallback(keyCmdNumber, 0)

	for i := 0; i < s.cmdNo; i++ {
		cmd := s.storage.String(keyCmd + strconv.Itoa(i))
		if cmd != "" {
			s.cmds = append(s.cmds, cmd)
		}
	}

	return &s
}
