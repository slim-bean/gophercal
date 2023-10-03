package imagen

import (
	"fmt"
	"image"
	"log"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/gouthamve/gophercal/todoist"
)

// The inkplate is 1200x825. 50% of it would be todoist, hence 600x825
// 75% for task name. 25% for the project
const (
	maxTasks             = 15
	todoWidth            = 600.0
	todoHeight           = 825.0
	outsideBoundaryWidth = 2.0
	innerBoundaryWidth   = 3.0
	lineWidth            = 2.0

	taskPortion    = 0.70
	projectPortion = 0.30
)

func GenerateTodoistImage(tasks []todoist.Task) image.Image {
	font, err := truetype.Parse(goregular.TTF)
	if err != nil {
		log.Fatal(err)
	}
	face := truetype.NewFace(font, &truetype.Options{Size: 20})

	if len(tasks) > maxTasks {
		tasks = tasks[:maxTasks]
	}

	tdCtx := gg.NewContext(todoWidth, todoHeight)
	tdCtx.SetFontFace(face)

	// White background
	tdCtx.DrawRectangle(0, 0, todoWidth, todoHeight)
	tdCtx.SetRGB(1, 1, 1)
	tdCtx.Fill()

	tdCtx.SetRGB(0, 0, 0)
	taskHeight := todoHeight / maxTasks
	for i, task := range tasks {
		// Draw a rectangle
		yStart := float64(i) * todoHeight / maxTasks
		rectangleWidth := todoWidth - 2*outsideBoundaryWidth

		tdCtx.SetLineWidth(lineWidth)
		tdCtx.DrawRoundedRectangle(outsideBoundaryWidth, yStart, rectangleWidth, taskHeight, 5)
		tdCtx.Stroke()

		textWidth := rectangleWidth - 2*innerBoundaryWidth
		taskWidth := textWidth * taskPortion
		projectWidth := textWidth * projectPortion

		taskName := truncateString(tdCtx, task.Content, float64(taskWidth))
		tdCtx.DrawStringAnchored(taskName, innerBoundaryWidth+outsideBoundaryWidth, yStart+taskHeight/2, 0, 0.5)

		projectSeparatorX := innerBoundaryWidth + outsideBoundaryWidth + taskWidth
		tdCtx.DrawLine(projectSeparatorX, yStart, projectSeparatorX, yStart+taskHeight)
		tdCtx.Stroke()

		projectName := truncateString(tdCtx, createProjectText(task), projectWidth)
		tdCtx.DrawStringAnchored(projectName, 2*innerBoundaryWidth+outsideBoundaryWidth+taskWidth, yStart+taskHeight/2, 0, 0.5)
	}

	return tdCtx.Image()
}

func truncateString(tdCtx *gg.Context, str string, maxWidth float64) string {
	strWidth, _ := tdCtx.MeasureString(str)
	if strWidth <= maxWidth {
		return str
	}

	for i := len(str) - 1; i >= 0; i-- {
		truncatedString := str[:i] + "..."
		w, _ := tdCtx.MeasureString(truncatedString)
		if w <= maxWidth {
			return truncatedString
		}
	}

	return ""
}

func createProjectText(task todoist.Task) string {
	return fmt.Sprintf("%s - %s", task.Project, task.Due.Format("Jan 2"))
}
