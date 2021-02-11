package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/stianeikeland/go-rpio"
)

type Light string

type LightJson struct {
	Light Light `json:"light"`
}

const (
	RedLight    Light = "red"
	YellowLight Light = "yellow"
	GreenLight  Light = "green"
	OffLight    Light = "off"

	RedPin    = rpio.Pin(16)
	YellowPin = rpio.Pin(20)
	GreenPin  = rpio.Pin(21)
	ButtonPin = rpio.Pin(26)
)

var light Light = OffLight

func preparePins() {
	RedPin.Output()
	YellowPin.Output()
	GreenPin.Output()
}

func turnOffLights() {
	RedPin.Low()
	YellowPin.Low()
	GreenPin.Low()
}

func setLight(currentLight *Light, updatedLight Light) error {
	switch updatedLight {
	case RedLight:
		*currentLight = RedLight
		turnOffLights()
		RedPin.High()
	case YellowLight:
		*currentLight = YellowLight
		turnOffLights()
		YellowPin.High()
	case GreenLight:
		*currentLight = GreenLight
		turnOffLights()
		GreenPin.High()
	case OffLight:
		*currentLight = OffLight
		turnOffLights()
	default:
		var errorMsg = fmt.Sprintf("Unknown light value: %s", updatedLight)
		return errors.New(errorMsg)
	}
	return nil
}

func cycleLight(currentLight *Light) {
	switch *currentLight {
	case RedLight:
		setLight(currentLight, OffLight)
	case YellowLight:
		setLight(currentLight, RedLight)
	case GreenLight:
		setLight(currentLight, YellowLight)
	case OffLight:
		setLight(currentLight, GreenLight)
	}
}

func status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case "GET":
		log.Printf("GET /status,%s", light)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{"light": "%s"}`, light)))

	case "PATCH":
		log.Printf("PATCH /status,%s", light)

		var updatedLight LightJson
		err := json.NewDecoder(r.Body).Decode(&updatedLight)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = setLight(&light, updatedLight.Light)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(fmt.Sprintf(`{"light": "%s"}`, light)))
	default:
		log.Print("Unknown Request")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "not found"}`))
	}
}

func shutdown() {
	RedPin.High()
	YellowPin.High()
	GreenPin.High()
	time.Sleep(1 * time.Second)
	rpio.Close()
	if err := exec.Command("sudo", "shutdown", "now").Run(); err != nil {
		fmt.Println("Failed to initiate shutdown:", err)
	}
}

func processButton() {
	pressed := false
	sleepInterval := 50 * time.Millisecond
	shutdownThres := 5 * time.Second
	timePressed := 0 * time.Millisecond
	for {
		time.Sleep(sleepInterval)
		res := ButtonPin.Read()
		// Nested if aren't cool. But this way we only check the pin once
		if res == rpio.High {
			if pressed == false {
				pressed = true
			} else {
				timePressed += sleepInterval
				if timePressed == shutdownThres {
					fmt.Println("Shutting down!")
					shutdown()
				}
			}
		} else if pressed == true {
			cycleLight(&light)
			pressed = false
			timePressed = 0
		}
	}
}

func cleanExit(ch chan os.Signal) {
	<-ch
	log.Print("Received signal. Shutting down server")
	turnOffLights()
	rpio.Close()
	os.Exit(1)
}

func main() {
	// Open and map memory to access gpio, check for errors
	if err := rpio.Open(); err != nil {
		log.Fatalf("Failed while opening the pins: %s", err)
		os.Exit(1)
	}
	preparePins()
	// Unmap gpio memory when done
	defer rpio.Close()

	// Exit cleanly when interrupted
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go cleanExit(signalChannel)

	go processButton()

	setLight(&light, GreenLight)

	var port = ":8080"
	log.Printf("Starting server on port %s", port)
	http.HandleFunc("/status", status)
	log.Fatal(http.ListenAndServe(port, nil))
}
