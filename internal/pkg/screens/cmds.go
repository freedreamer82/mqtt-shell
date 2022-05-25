package screens

import (
	"fmt"
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
		}
		s.selectedCmd = -1
		s.listData.Refresh()

	})

	s.listData = widget.NewList(
		func() int {
			return len(s.cmds)
		},
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewLabel(""), layout.NewSpacer(), cmdDeleteButton)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*fyne.Container).Objects[0].(*widget.Label).SetText(s.cmds[id])
		},
	)
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

func NewCmdOverlay(app fyne.Window) *CmdScreen {

	s := CmdScreen{onCLoseCb: nil}
	s.selectedCmd = -1
	s.app = app

	return &s
}
