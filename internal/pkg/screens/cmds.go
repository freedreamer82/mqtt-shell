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
	container   fyne.CanvasObject
	cmds        []string
	selectedCmd int
	inputcmd    binding.String
	listData    *widget.List
	app         fyne.Window
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

func (s *CmdScreen) ShowPopUp() {
	s.selectedCmd = -1

	cancelButton := widget.NewButton("Cancel", func() {
		s.container.Hide()
	})

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
		fmt.Printf("selected %d", id)
		//popup.Hide()
		s.selectedCmd = id
	}

	c := container.NewBorder(nil, cancelButton, nil, nil,
		container.NewScroll(s.listData))

	s.container = widget.NewModalPopUp(c, s.app.Canvas())

	s.container.Resize(fyne.NewSize(s.app.Canvas().Size().Width/2, s.app.Canvas().Size().Height/2))
	s.container.Show()
}

func NewCmdOverlay(app fyne.Window) *CmdScreen {

	s := CmdScreen{}
	s.selectedCmd = -1
	s.app = app

	return &s
}
