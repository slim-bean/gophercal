package imagen

import (
	"image"

	"github.com/fogleman/gg"
)

func MergeImages(todoImg, gcalImg image.Image) image.Image {
	finalCtx := gg.NewContext(todoWidth+calWidth, todoHeight)
	finalCtx.DrawImage(todoImg, 0, 0)
	finalCtx.DrawImage(gcalImg, todoWidth, 0)

	return finalCtx.Image()
}
