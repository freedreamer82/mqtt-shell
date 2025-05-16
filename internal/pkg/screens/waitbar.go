package screens

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type WaitBar struct {
	progressBar      *widget.ProgressBarInfinite
	progressBarPopUp *widget.PopUp
	window           fyne.Window
}

func NewWaitBar(window fyne.Window) *WaitBar {
	w := WaitBar{progressBar: widget.NewProgressBarInfinite()}
	w.window = window
	w.progressBarPopUp = widget.NewModalPopUp(w.progressBar, window.Canvas())

	w.Hide()

	return &w
}

func (w *WaitBar) Show() {

	w.progressBar.Start()
	w.progressBarPopUp.Show()
}

func (w *WaitBar) Visible() bool {

	return w.progressBarPopUp.Visible()
}

func (w *WaitBar) Hide() {

	w.progressBar.Stop()
	w.progressBarPopUp.Hide()

}

func (w *WaitBar) Resize(size fyne.Size) {
	w.progressBarPopUp.Resize(size)
}
