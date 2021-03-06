package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis"

	"net/http"

	"github.com/go-chi/render"
	"github.com/securingsincity/gbridge"

	"github.com/go-chi/chi"

	"github.com/lorenzobenvenuti/ifttt"
	rpio "github.com/stianeikeland/go-rpio"
)

type TimingPin struct {
	LastVibrationChange time.Time
	IsVibrating         bool
}

// WithRole returns a pointer to a copy of Client (c *Client) with c.config.Role set to the new role.
func (t *TimingPin) isStillVibrating(now time.Time) bool {
	return now.Sub(t.LastVibrationChange).Seconds() < float64(60)
}

func (t *TimingPin) vibrating(now time.Time) {
	t.IsVibrating = true
	t.LastVibrationChange = now
}

func (t *TimingPin) vibratingStopped(now time.Time) {
	t.IsVibrating = false
	t.LastVibrationChange = now
}

var iftttKey = os.Getenv("IFTTT_KEY")
var iftttEvent = os.Getenv("MAKER_EVENT_NAME")
var googleClientID = os.Getenv("GOOGLE_CLIENT_ID")
var googleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")

func sendIftttMessage(client ifttt.IftttClient, event string, message string) {
	fmt.Printf("Sending message - %v - %v \n", event, message)
	client.Trigger(event, []string{message})
}

func buildHandler(redisClient *redis.Client) func(http.ResponseWriter, *http.Request) {
	fn := func(w http.ResponseWriter, r *http.Request) {
		value, err := redisClient.Get("isVibrating").Result()
		if err != nil {
			panic(err)
		}
		intVal, err := strconv.Atoi(value)
		if err != nil {
			panic(err)
		}
		render.JSON(w, r, map[string]bool{
			"isVibrating": intVal == 1,
		})
	}
	return fn
}

func buildHandleQuery(redisClient *redis.Client) func(dev gbridge.Device, res *gbridge.DeviceState) {
	fn := func(dev gbridge.Device, res *gbridge.DeviceState) {
		value, err := redisClient.Get("isVibrating").Result()
		if err != nil {
			panic(err)
		}
		intVal, err := strconv.Atoi(value)
		if err != nil {
			panic(err)
		}
		isVibrating := intVal == 1
		res.Online = true
		res.On = true
		res.IsPaused = !isVibrating
		res.IsRunning = isVibrating
		log.Printf("Query Res: %+v\n", res)
	}
	return fn
}

func main() {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	iftttClient := ifttt.NewIftttClient(iftttKey)
	err := rpio.Open()

	if err != nil {
		fmt.Errorf("Failure to open")
	}
	defer rpio.Close()
	pinNumber := os.Getenv("PIN")
	pinNumberAsInt, err := strconv.Atoi(pinNumber)

	if err != nil {
		fmt.Errorf("Pin number is not an integer")
	}
	pin := rpio.Pin(pinNumberAsInt)
	pin.Input()
	pin.PullDown() // Input mode
	pin.Detect(rpio.RiseEdge)
	timingPin := TimingPin{
		LastVibrationChange: time.Now(),
		IsVibrating:         false,
	}
	go func() {

		r := chi.NewRouter()
		b := gbridge.Bridge{
			ClientId:     googleClientID,
			ClientSecret: googleClientSecret,
		}
		b.HandleQuery(CreateWasherDryer("1", "Dryer"), buildHandleQuery(client))
		r.Route("/", func(r chi.Router) {
			r.Get("/status", buildHandler(client))
			r.Get("/oauth", b.HandleOauth)
			r.Post("/smarthome", b.HandleSmartHome)
			r.HandleFunc("/token", b.HandleToken)
		})
		log.Fatal(http.ListenAndServe(":3000", r))
	}()
	c := time.Tick(5 * time.Second)
	for now := range c {
		edgeDetected := pin.EdgeDetected()
		if !timingPin.isStillVibrating(now) && !timingPin.IsVibrating && edgeDetected {
			// we're vibrating now. so let's send off a notification
			fmt.Printf("Starting up - %v \n", now)
			timingPin.vibrating(now)
			client.Set("isVibrating", 1, 0).Result()
			go sendIftttMessage(iftttClient, iftttEvent, "Started")
		} else if !timingPin.isStillVibrating(now) && timingPin.IsVibrating {
			//we're not vibrating anymore so lets send off a notification
			fmt.Printf("Stopping - %v \n", now)
			timingPin.vibratingStopped(now)
			client.Set("isVibrating", 0, 0).Result()
			go sendIftttMessage(iftttClient, iftttEvent, "Stopped")
		} else if edgeDetected {
			// we're still vibrating let's keep it going.
			timingPin.vibrating(now)
		}
	}

}

func CreateWasherDryer(id string, name string) gbridge.Device {

	d := gbridge.Device{
		Id:     id,
		Type:   gbridge.DeviceTypeDryer,
		Traits: []gbridge.DeviceTrait{gbridge.DeviceTraitStartStop},
		Attributes: gbridge.Attributes{
			Pausable: false,
		},
		Name: gbridge.DeviceName{
			DefaultNames: []string{name},
			Name:         name,
			Nicknames:    []string{name},
		},
	}
	return d
}
