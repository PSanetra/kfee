package pages

import (
	"kfee/ui"
	"kfee/ui/components/statusbar"
)

type Page interface {
	ui.View
	statusbar.Provider
}
