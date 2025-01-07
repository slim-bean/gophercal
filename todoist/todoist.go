package todoist

import (
	"sort"
	"time"

	"github.com/volyanyk/todoist"
)

type Todoist struct {
	client *todoist.Client
	filter string
}

type Task struct {
	Id      string
	Project string
	Section string
	Content string
	Due     time.Time
}

func New(token, filter string) Todoist {
	return Todoist{
		client: todoist.New(token),
		filter: filter,
	}
}

func (t Todoist) GetTodaysTasks() ([]Task, error) {
	apiTasks, err := t.client.GetActiveTasks(todoist.GetActiveTasksRequest{
		Filter: t.filter,
	})
	if err != nil {
		return nil, err
	}

	projects := map[string]string{} // map from id to name
	sections := map[string]string{}
	for _, task := range *apiTasks {
		if _, ok := projects[task.ProjectId]; !ok {
			project, err := t.client.GetProjectById(task.ProjectId)
			if err != nil {
				return nil, err
			}

			projects[project.ID] = project.Name
		}
	}

	for _, task := range *apiTasks {
		if task.SectionId == "" {
			continue
		}
		if _, ok := sections[task.SectionId]; !ok {
			section, err := t.client.GetSectionById(task.SectionId)
			if err != nil {
				return nil, err
			}

			sections[section.ID] = section.Name
		}
	}

	tasks := make([]Task, 0, len(*apiTasks))

	for _, task := range *apiTasks {
		due := time.Now().Add(24 * time.Hour)
		if task.Due != nil {
			due, err = time.ParseInLocation(time.DateOnly, task.Due.Date, time.Local)
			if err != nil {
				return nil, err
			}
		}

		tasks = append(tasks, Task{
			Id:      task.Id,
			Project: projects[task.ProjectId],
			Section: sections[task.SectionId],
			Content: task.Content,
			Due:     due,
		})
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Due.Before(tasks[j].Due)
	})

	return tasks, nil
}
