package todoist

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"
)

const baseURL = "https://api.todoist.com/api/v1"

type Todoist struct {
	token  string
	filter string
	sortBy string
	client *http.Client
}

type Task struct {
	Id      string
	Project string
	Section string
	Content string
	Due     time.Time
	Order   int
}

// API response types

type apiDue struct {
	Date string `json:"date"`
}

type apiTask struct {
	Id        string  `json:"id"`
	ProjectId string  `json:"project_id"`
	SectionId string  `json:"section_id"`
	Content   string  `json:"content"`
	Due       *apiDue `json:"due"`
	Order     int     `json:"child_order"`
}

type apiTasksResponse struct {
	Results []apiTask `json:"results"`
}

type apiProject struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type apiSection struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func New(token, filter, sortBy string) Todoist {
	return Todoist{
		token:  token,
		filter: filter,
		sortBy: sortBy,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t Todoist) doGet(endpoint string, query url.Values, out interface{}) error {
	u := baseURL + endpoint
	if query != nil {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.token)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("todoist API %s returned status %d", endpoint, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}

func (t Todoist) getFilteredTasks() ([]apiTask, error) {
	q := url.Values{}
	q.Set("query", t.filter)

	var result apiTasksResponse
	if err := t.doGet("/tasks/filter", q, &result); err != nil {
		return nil, fmt.Errorf("getting filtered tasks: %w", err)
	}

	return result.Results, nil
}

func (t Todoist) getProjectByID(id string) (apiProject, error) {
	var project apiProject
	if err := t.doGet("/projects/"+id, nil, &project); err != nil {
		return project, fmt.Errorf("getting project %s: %w", id, err)
	}
	return project, nil
}

func (t Todoist) getSectionByID(id string) (apiSection, error) {
	var section apiSection
	if err := t.doGet("/sections/"+id, nil, &section); err != nil {
		return section, fmt.Errorf("getting section %s: %w", id, err)
	}
	return section, nil
}

func (t Todoist) GetTodaysTasks() ([]Task, error) {
	apiTasks, err := t.getFilteredTasks()
	if err != nil {
		return nil, err
	}

	projects := map[string]string{} // map from id to name
	sections := map[string]string{}

	for _, task := range apiTasks {
		if _, ok := projects[task.ProjectId]; !ok {
			project, err := t.getProjectByID(task.ProjectId)
			if err != nil {
				return nil, err
			}
			projects[project.Id] = project.Name
		}
	}

	for _, task := range apiTasks {
		if task.SectionId == "" {
			continue
		}
		if _, ok := sections[task.SectionId]; !ok {
			section, err := t.getSectionByID(task.SectionId)
			if err != nil {
				return nil, err
			}
			sections[section.Id] = section.Name
		}
	}

	tasks := make([]Task, 0, len(apiTasks))

	for _, task := range apiTasks {
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
			Order:   task.Order,
		})
	}

	switch t.sortBy {
	case "order":
		sort.Slice(tasks, func(i, j int) bool {
			return tasks[i].Order < tasks[j].Order
		})
	default: // "due-date"
		sort.Slice(tasks, func(i, j int) bool {
			return tasks[i].Due.Before(tasks[j].Due)
		})
	}

	return tasks, nil
}
