package main

import (
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

	"github.com/gouthamve/gophercal/gcalendar"
	"github.com/gouthamve/gophercal/imagen"
	"github.com/gouthamve/gophercal/todoist"
)

var gopherCal struct {
	Run struct {
		TodoistToken  string `kong:"required,env='TODOIST_TOKEN',help='Todoist API token'"`
		GCalCredsFile string `kong:"help='Google Calendar credentials file',default='credentials.json',name='gcal-credentials-file'"`
		GCalTokenFile string `kong:"help='Google Calendar token file',default='token.json',name='gcal-token-file'"`
		GCalEmail     string `kong:"required,help='Google Calendar email address',name='gcal-email'"`
	} `cmd:""`
}

func main() {
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
		td := todoist.New(gopherCal.Run.TodoistToken)
		calendar, err := gcalendar.NewCalendar(gopherCal.Run.GCalCredsFile, gopherCal.Run.GCalTokenFile, gopherCal.Run.GCalEmail)
		checkErr(err)

		http.Handle("/dash.jpg", promhttp.InstrumentHandlerDuration(durationHistogram.MustCurryWith(prometheus.Labels{"handler": "dash.jpg"}), http.HandlerFunc(dashHandler(td, calendar))))
		http.Handle("/metrics", promhttp.Handler())
		// http.HandleFunc("/refresh-auth", authHandler("credentials.json", "token.json"))

		log.Println("Listening on :8364")
		log.Fatal(http.ListenAndServe(":8364", nil))
	}
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func dashHandler(td todoist.Todoist, calendar *gcalendar.Calendar) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		mergedImg, err := generateImage(td, calendar)
		if err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := gg.SaveJPG("dash.jpg", mergedImg, 50); err != nil {
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

// TODO: Implement authHandler to refresh tokens online.
// func authHandler(credsFile string, tokenFile string) func(w http.ResponseWriter, r *http.Request) {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		b, err := os.ReadFile(credsFile)
// 		if err != nil {
// 			err = fmt.Errorf("unable to read client secret file: %w", err)
// 			http.Error(w, err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
// 		if err != nil {
// 			err = fmt.Errorf("unable to parse client secret file to config: %w", err)
// 			http.Error(w, err.Error(), http.StatusInternalServerError)
// 			return
// 		}

// 		if r.URL.Query().Get("code") != "" {
// 			authCode := r.URL.Query().Get("code")
// 			fmt.Println("Got auth code: ", authCode)
// 			tok, err := config.Exchange(context.Background(), authCode)
// 			if err != nil {
// 				err = fmt.Errorf("unable to get token: %w", err)
//              TODO: fails with "unable to get token: oauth2: "invalid_grant" "Bad Request"". This could be because we are creating a new
//              config. object on refresh.
// 				http.Error(w, err.Error(), http.StatusInternalServerError)
// 				return
// 			}

// 			saveToken(tokenFile, tok)
// 			w.Write([]byte("Successfully authenticated. You can close this tab now."))
// 			return
// 		}

// 		config.RedirectURL += ":8364/refresh-auth"
// 		authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
// 		http.Redirect(w, r, authURL, http.StatusFound)
// 	}
// }

func generateImage(td todoist.Todoist, calendar *gcalendar.Calendar) (image.Image, error) {
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

	gcalImg := imagen.GenerateCalendarImage(events)

	log.Println("events image generated")

	// Merge the two images
	mergedImg := imagen.MergeImages(todoistImg, gcalImg)

	log.Println("images merged")

	return mergedImg, nil
}

// Saves a token to a file path.
// func saveToken(path string, token *oauth2.Token) {
// 	fmt.Printf("Saving credential file to: %s\n", path)
// 	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
// 	if err != nil {
// 		log.Fatalf("Unable to cache oauth token: %v", err)
// 	}
// 	defer f.Close()
// 	json.NewEncoder(f).Encode(token)
// }
