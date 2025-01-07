package gcalendar

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

const (
	calendarName = "primary"
	// timeZone     = "Europe/Berlin"
)

var (
	clientCallHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gophercal_googleapi_request_duration_seconds",
			Help:    "A histogram of request latencies.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"code", "method"},
	)
)

type Event struct {
	Start time.Time
	End   time.Time

	Title string
}

type Calendar struct {
	srv *calendar.Service

	email    string
	location string
}

func NewCalendar(config *oauth2.Config, tokenFile, email, location string) (*Calendar, error) {
	ctx := context.Background()

	client, err := getClient(config, tokenFile)
	if err != nil {
		return nil, err
	}

	client.Transport = promhttp.InstrumentRoundTripperDuration(clientCallHistogram, client.Transport)

	srv, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	return &Calendar{srv: srv, email: email}, err
}

func (c Calendar) Events() ([]Event, error) {
	startTime := time.Now().Add(-6 * time.Hour)
	endTime := startTime.Add(15 * time.Hour)

	calEvents, err := c.srv.Events.List(calendarName).
		TimeMin(startTime.Format(time.RFC3339)).
		TimeMax(endTime.Format(time.RFC3339)).
		ShowDeleted(false).
		SingleEvents(true).
		OrderBy("startTime").
		Do()
	if err != nil {
		return nil, err
	}

	var events []Event
	for _, item := range calEvents.Items {
		// skip the ones I said no to.
		skip := false
		for _, attendee := range item.Attendees {
			if attendee.Email == c.email && attendee.ResponseStatus == "declined" {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if item.EventType == "workingLocation" {
			continue
		}
		if item.Start.DateTime == "" || item.End.DateTime == "" {
			continue
		}

		startTime, err := time.Parse(time.RFC3339, item.Start.DateTime)
		if err != nil {
			return nil, err
		}
		endTime, err := time.Parse(time.RFC3339, item.End.DateTime)
		if err != nil {
			return nil, err
		}

		loc := time.Local
		if c.location != "" {
			loc, err = time.LoadLocation(c.location)
			if err != nil {
				log.Fatal(err)
			}
		}

		events = append(events, Event{Start: startTime.In(loc), End: endTime.In(loc), Title: item.Summary})
	}

	return events, nil
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config, tokenFile string) (*http.Client, error) {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tok, err := tokenFromFile(tokenFile)
	if err != nil {
		return nil, err
	}

	return oauth2.NewClient(context.Background(), newPersistingTokenSource(config.TokenSource(context.Background(), tok), tokenFile)), nil
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func persistTokenToFile(tok *oauth2.Token, file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(tok)
}

type persistingTokenSource struct {
	src       oauth2.TokenSource
	file      string
	currentTk *oauth2.Token
}

func newPersistingTokenSource(src oauth2.TokenSource, file string) *persistingTokenSource {
	return &persistingTokenSource{src: src, file: file}
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tk, err := p.src.Token()
	if err != nil {
		return nil, err
	}

	if p.currentTk == nil || p.currentTk.AccessToken != tk.AccessToken {
		if err := persistTokenToFile(tk, p.file); err != nil {
			return nil, err
		}
		p.currentTk = tk
	}

	return tk, nil
}
