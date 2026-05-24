package common

const (
	StyleBold          = "b"
	StyleItalic        = "i"
	StyleUnderline     = "u"
	StyleStrikethrough = "s"
	StyleListUnordered = "lst_1"
	StyleListOrdered   = "lst_2"
)

// Style is one positional run of formatting over the final plain-text body.
// Positions are UTF-16 code units (matches Zalo client / zca-js wire shape).
type Style struct {
	Start int    `json:"start"`
	Len   int    `json:"len"`
	St    string `json:"st"`
}
