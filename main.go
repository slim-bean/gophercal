package main

import (
	"os"

	"github.com/fogleman/gg"
	"github.com/gouthamve/inkplate-adventures/imagen"
	"github.com/gouthamve/inkplate-adventures/todoist"
)

func main() {
	td := todoist.New(os.Getenv("TODOIST_TOKEN"))

	tasks, err := td.GetTodaysTasks()
	checkErr(err)
	img := imagen.GenerateTodoistImage(tasks)

	checkErr(gg.SavePNG("out.png", img))
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
