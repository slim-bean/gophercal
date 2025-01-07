package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/alecthomas/kong"
	"github.com/fogleman/gg"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"

	"github.com/gouthamve/gophercal/gcalendar"
	"github.com/gouthamve/gophercal/imagen"
	"github.com/gouthamve/gophercal/todoist"
)

var gopherCal struct {
	Run struct {
		TodoistToken  string `kong:"required,env='TODOIST_TOKEN',help='Todoist API token'"`
		GCalCredsFile string `kong:"help='Google Calendar credentials file',default='credentials.json',name='gcal-credentials-file'"`
		GCalTokenFile string `kong:"help='Where to save Google Calendar token file',default='token.json',name='gcal-token-file'"`
		GCalEmail     string `kong:"required,help='Google Calendar email address',name='gcal-email'"`

		TodoistFilter string `kong:"help='Todoist filter to use',default='(today | overdue)',name='todoist-filter'"`
		Location      string `kong:"help='Location to use for weather',default='',name='location'"`
	} `cmd:""`
}

func main() {
	log.Println("Starting gophercal")
	durationHistogram := promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gophercal_request_duration_seconds",
			Help:    "A histogram of latencies for requests.",
			Buckets: []float64{.25, .5, 1, 2.5, 5, 10},
		},
		[]string{"handler", "method", "code"},
	)

	ctx := kong.Parse(&gopherCal,
		kong.Name("gophercal"),
		kong.Description("A Google Calendar and Todoist image generator for eInk devices."),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}))

	switch ctx.Command() {
	case "run":
		td := todoist.New(gopherCal.Run.TodoistToken, gopherCal.Run.TodoistFilter)

		b, err := os.ReadFile(gopherCal.Run.GCalCredsFile)
		if err != nil {
			err = fmt.Errorf("unable to read client secret file: %w", err)
			checkErr(err)
		}
		config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
		if err != nil {
			err = fmt.Errorf("unable to parse client secret file to config: %w", err)
			checkErr(err)
		}

		var calendar *gcalendar.Calendar
		http.Handle("/dash.jpg", promhttp.InstrumentHandlerDuration(durationHistogram.MustCurryWith(prometheus.Labels{"handler": "dash.jpg"}), http.HandlerFunc(dashHandler(config, td, calendar, gopherCal.Run.Location))))
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/refresh-auth", authHandler(config, gopherCal.Run.GCalTokenFile))

		log.Println("Listening on :8364")
		log.Fatal(http.ListenAndServe(":8364", nil))
	}
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func dashHandler(config *oauth2.Config, td todoist.Todoist, calendar *gcalendar.Calendar, location string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if calendar == nil {
			log.Println("making new calendar object")
			_, err := os.Stat(gopherCal.Run.GCalTokenFile)
			if err != nil {
				if os.IsNotExist(err) {
					log.Println("Token file does not exist. open /refresh-auth to create a token file")
				}
				log.Println(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}

			calendar, err = gcalendar.NewCalendar(config, gopherCal.Run.GCalTokenFile, gopherCal.Run.GCalEmail, gopherCal.Run.Location)
			if err != nil {
				log.Println(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		mergedImg, err := generateImage(td, calendar, location)
		if err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := gg.SaveJPG("dash.jpg", mergedImg, 80); err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		buf, err := os.ReadFile("dash.jpg")
		if err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/jpg")
		w.Header().Set("Content-Length", strconv.Itoa(len(buf)))
		w.Write(buf)
	}
}

func authHandler(config *oauth2.Config, tokenFile string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		if r.URL.Query().Get("code") != "" {
			authCode := r.URL.Query().Get("code")
			fmt.Println("Got auth code: ", authCode)
			tok, err := config.Exchange(context.Background(), authCode)
			if err != nil {
				err = fmt.Errorf("unable to get token: %w", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			saveToken(tokenFile, tok)
			w.Write([]byte("Successfully authenticated. You can close this tab now."))
			return
		}

		// offline and forced approval are requried to get a refresh token in the response, if you were already logged in you wouldn't get a refresh token.
		authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

func generateImage(td todoist.Todoist, calendar *gcalendar.Calendar, location string) (image.Image, error) {
	log.Println("Starting ")
	tasks, err := td.GetTodaysTasks()
	if err != nil {
		return nil, fmt.Errorf("error getting todoist tasks: %w", err)
	}

	todoistImg := imagen.GenerateTodoistImage(tasks)

	log.Println("Tasks image generated")

	events, err := calendar.Events()
	if err != nil {
		return nil, fmt.Errorf("error getting gcal events: %w", err)
	}

	log.Println("events retrieved")

	gcalImg := imagen.GenerateCalendarImage(events, location)

	log.Println("events image generated")

	// Merge the two images
	mergedImg := imagen.MergeImages(todoistImg, gcalImg)

	log.Println("images merged")

	return mergedImg, nil
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}
