// Package config loads the flight's identity, plan, and runtime settings from a
// plan file and environment variables.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/cloud-native-airlines/flight/internal/flight"
)

// Config is the resolved startup configuration.
type Config struct {
	Plan       flight.Plan
	RunID      string // optional; empty means bind to the first tick
	ListenAddr string

	// Integration (all optional; empty disables that hop, keeping Phase 1 local).
	NATSURL          string        // subscribe to Simulator ticks when set
	TickSubject      string        // NATS subject for ticks
	ADSBURL          string        // post reports to ADS-B when set
	OperationsURL    string        // post lifecycle events to Operations when set
	FlightInstanceID string        // the Operations flight-instance this leg executes
	ReportTimeout    time.Duration // per-attempt deadline for ADS-B / Operations delivery
	ReportRetries    int           // retries after the first attempt
}

// airport mirrors the origin/destination shape of the shared scenario files.
type airport struct {
	IATA         string  `json:"iata"`
	LatitudeDeg  float64 `json:"latitude_deg"`
	LongitudeDeg float64 `json:"longitude_deg"`
}

type planFile struct {
	FlightID        string    `json:"flight_id"`
	AircraftID      string    `json:"aircraft_id"`
	Origin          airport   `json:"origin"`
	Destination     airport   `json:"destination"`
	DepartureTime   time.Time `json:"departure_time"`
	ArrivalTime     time.Time `json:"arrival_time"`
	CruiseAltitudeM *float64  `json:"cruise_altitude_m,omitempty"`
	ClimbFraction   *float64  `json:"climb_fraction,omitempty"`
	DescentFraction *float64  `json:"descent_fraction,omitempty"`
}

// Load resolves configuration from the environment. The plan comes from either
// FLIGHT_PLAN_JSON (an inline plan document, used when a dispatcher launches the
// leg) or FLIGHT_PLAN (a path to a plan JSON); the inline form wins when both are
// set. One of the two is required.
func Load() (Config, error) {
	var raw []byte
	if inline := os.Getenv("FLIGHT_PLAN_JSON"); inline != "" {
		raw = []byte(inline)
	} else {
		path := os.Getenv("FLIGHT_PLAN")
		if path == "" {
			return Config{}, fmt.Errorf("one of FLIGHT_PLAN_JSON or FLIGHT_PLAN is required (a flight-plan JSON)")
		}
		var err error
		if raw, err = os.ReadFile(path); err != nil {
			return Config{}, fmt.Errorf("reading FLIGHT_PLAN: %w", err)
		}
	}

	var pf planFile
	if err := json.Unmarshal(raw, &pf); err != nil {
		return Config{}, fmt.Errorf("parsing flight plan: %w", err)
	}

	plan := flight.Plan{
		FlightID:    pf.FlightID,
		AircraftID:  pf.AircraftID,
		Origin:      flight.Point{Lat: pf.Origin.LatitudeDeg, Lon: pf.Origin.LongitudeDeg},
		Destination: flight.Point{Lat: pf.Destination.LatitudeDeg, Lon: pf.Destination.LongitudeDeg},
		Departure:   pf.DepartureTime.UTC(),
		Arrival:     pf.ArrivalTime.UTC(),
		// Defaults; overridden by the plan file, then the environment below.
		CruiseAltitudeM: 10668,
		ClimbFraction:   0.25,
		DescentFraction: 0.25,
	}
	if pf.CruiseAltitudeM != nil {
		plan.CruiseAltitudeM = *pf.CruiseAltitudeM
	}
	if pf.ClimbFraction != nil {
		plan.ClimbFraction = *pf.ClimbFraction
	}
	if pf.DescentFraction != nil {
		plan.DescentFraction = *pf.DescentFraction
	}

	var err error
	if plan.CruiseAltitudeM, err = envFloat("FLIGHT_CRUISE_ALTITUDE_M", plan.CruiseAltitudeM); err != nil {
		return Config{}, err
	}
	if plan.ClimbFraction, err = envFloat("FLIGHT_CLIMB_FRACTION", plan.ClimbFraction); err != nil {
		return Config{}, err
	}
	if plan.DescentFraction, err = envFloat("FLIGHT_DESCENT_FRACTION", plan.DescentFraction); err != nil {
		return Config{}, err
	}

	if err := plan.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid flight plan: %w", err)
	}

	operationsURL := os.Getenv("OPERATIONS_URL")
	flightInstanceID := os.Getenv("FLIGHT_INSTANCE_ID")
	if operationsURL != "" && flightInstanceID == "" {
		return Config{}, fmt.Errorf("FLIGHT_INSTANCE_ID is required when OPERATIONS_URL is set")
	}

	return Config{
		Plan:             plan,
		RunID:            os.Getenv("FLIGHT_RUN_ID"),
		ListenAddr:       envString("LISTEN_ADDR", "0.0.0.0:8080"),
		NATSURL:          os.Getenv("NATS_URL"),
		TickSubject:      envString("SIM_TICK_SUBJECT", "cna.sim.tick"),
		ADSBURL:          os.Getenv("ADSB_URL"),
		OperationsURL:    operationsURL,
		FlightInstanceID: flightInstanceID,
		ReportTimeout:    5 * time.Second,
		ReportRetries:    3,
	}, nil
}

func envString(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func envFloat(name string, def float64) (float64, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number: %w", name, err)
	}
	return f, nil
}
