package imagen

import (
	"image"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
)

func MergeImages(todoImg, gcalImg image.Image) image.Image {
	finalCtx := gg.NewContext(todoWidth+calWidth, todoHeight)
	finalCtx.DrawImage(todoImg, 0, 0)
	finalCtx.DrawImage(gcalImg, todoWidth, 0)

	return finalCtx.Image()
}

// stripUnrenderable removes any characters that the given font cannot
// render, avoiding empty-box glyphs on the e-ink display.
func stripUnrenderable(f *truetype.Font, s string) string {
	var b strings.Builder
	for _, r := range s {
		if f.Index(r) != 0 {
			b.WriteRune(r)
		}
	}
	result := b.String()
	// Collapse multiple spaces left behind by removed characters.
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	return strings.TrimSpace(result)
}
