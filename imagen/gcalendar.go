package imagen

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"time"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/gouthamve/gophercal/gcalendar"
	"golang.org/x/image/font/gofont/goregular"
)

// The inkplate is 1200x825. 50% of it would be calendar, hence 600x825
const (
	maxHours  = 8
	calWidth  = 600.0
	calHeight = 825.0

	hourHeight float64 = calHeight / maxHours
)

func GenerateCalendarImage(events []gcalendar.Event) image.Image {
	font, err := truetype.Parse(goregular.TTF)
	if err != nil {
		log.Fatal(err)
	}
	face := truetype.NewFace(font, &truetype.Options{Size: 20})

	calCtx := gg.NewContext(calWidth, calHeight)
	calCtx.SetFontFace(face)

	// White background
	calCtx.DrawRectangle(0, 0, calWidth, calHeight)
	calCtx.SetRGB(1, 1, 1)
	calCtx.Fill()

	calCtx.SetRGB(0, 0, 0)
	// Start from the previous hour.
	hours, minutes, _ := time.Now().Clock()
	startHour := hours - 1

	// Draw the line for the current time.
	calCtx.SetLineWidth(lineWidth * 1.5)
	yStart := float64(hours-startHour)*hourHeight + float64(minutes)/60*hourHeight
	calCtx.SetDash(10, 7)
	calCtx.DrawLine(0, yStart, calWidth, yStart)
	calCtx.Stroke()
	// TODO: Fill the dashes with white color for better visibility.
	// calCtx.SetRGB(1, 1, 1)
	// calCtx.SetLineWidth(lineWidth)
	// calCtx.SetDash(7, 7)

	calCtx.SetDash()

	// Draw the hour lines.
	for i := 0; i < maxHours; i++ {
		yStart := float64(i) * calHeight / maxHours
		timeFace := truetype.NewFace(font, &truetype.Options{Size: 25})
		calCtx.SetFontFace(timeFace)
		calCtx.DrawStringAnchored(fmt.Sprintf("%d:00", (startHour+i)%24), 0, yStart, 0, 1)
		calCtx.SetFontFace(face)

		rectangleWidth := calWidth - 2*outsideBoundaryWidth
		calCtx.SetLineWidth(lineWidth)
		calCtx.DrawRoundedRectangle(outsideBoundaryWidth, yStart, rectangleWidth, hourHeight, 5)
		calCtx.Stroke()
	}
	// Group by overlapping events.
	overlappingEvents := [][]gcalendar.Event{}

	for _, event := range events {
		overlapping := false
		for i, group := range overlappingEvents {
			if group[0].End.Before(event.Start) || group[0].End.Equal(event.Start) {
				continue
			}
			overlappingEvents[i] = append(group, event)
			overlapping = true
			break
		}
		if !overlapping {
			overlappingEvents = append(overlappingEvents, []gcalendar.Event{event})
		}
	}

	// Draw the events in the rectangles.
	for _, events := range overlappingEvents {
		for i, event := range events {
			overlaps := len(events)
			if overlaps > 3 {
				overlaps = 3
			}

			hourDiff := event.Start.Hour() - startHour
			// This means its a new day, so add the difference.
			if hourDiff < 0 {
				hourDiff += 24
			}

			yStart := float64(hourDiff) * hourHeight
			yStart += float64(event.Start.Minute()) / 60 * hourHeight

			img := drawEvent(event, overlaps)

			calCtx.DrawImage(img, int(innerBoundaryWidth)+i*calWidth/len(events), int(yStart))
		}
	}

	return calCtx.Image()
}

func drawEvent(event gcalendar.Event, overlaps int) image.Image {
	font, err := truetype.Parse(goregular.TTF)
	if err != nil {
		log.Fatal(err)
	}
	face := truetype.NewFace(font, &truetype.Options{Size: 20})

	hourHeight := calHeight / maxHours
	height := event.End.Sub(event.Start).Minutes() / 60 * hourHeight

	width := (calWidth - 2*outsideBoundaryWidth) / overlaps

	evCtx := gg.NewContext(width, int(height))
	evCtx.SetLineWidth(lineWidth / 3)
	evCtx.SetFontFace(face)

	// background
	evCtx.DrawRectangle(0, 0, calWidth, calHeight)
	evCtx.SetColor(color.RGBA{0, 0, 0, 60})
	evCtx.Fill()

	evCtx.SetRGB(0, 0, 0)
	evCtx.DrawRoundedRectangle(0, 0, float64(width), height, 5)
	evCtx.Stroke()

	eventName := truncateString(evCtx, event.Title, float64(width))
	evCtx.DrawStringAnchored(eventName, float64(width)/2, height/2, 0.5, 0.5)
	evCtx.Stroke()

	return evCtx.Image()
}
