package input

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/ui/primitives"
)

const inputDefaultWidth = unit.Dp(400)
const inputMinWidth = unit.Dp(160)

func inputField(gtx layout.Context, widget layout.Widget) layout.Dimensions {
	return primitives.Field(gtx, palette.Surface, widget)
}

func editorField(gtx layout.Context, widget layout.Widget) layout.Dimensions {
	primitives.ConstrainEditorWidth(&gtx, inputDefaultWidth, inputMinWidth)
	return inputField(gtx, widget)
}
