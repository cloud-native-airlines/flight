package config

import (
	"testing"
)

const inlinePlan = `{
  "flight_id": "CNA1000",
  "aircraft_id": "N100CA",
  "origin":      { "iata": "RST", "latitude_deg": 43.9083, "longitude_deg": -92.5000 },
  "destination": { "iata": "MSP", "latitude_deg": 44.8848, "longitude_deg": -93.2223 },
  "departure_time": "2026-10-06T14:00:00Z",
  "arrival_time":   "2026-10-06T14:40:00Z"
}`

func TestLoad_InlinePlanJSON(t *testing.T) {
	t.Setenv("FLIGHT_PLAN_JSON", inlinePlan)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with inline plan: %v", err)
	}
	if cfg.Plan.FlightID != "CNA1000" || cfg.Plan.AircraftID != "N100CA" {
		t.Errorf("unexpected plan identity: %+v", cfg.Plan)
	}
	if cfg.Plan.Origin.Lat != 43.9083 || cfg.Plan.Destination.Lon != -93.2223 {
		t.Errorf("coordinates not mapped: %+v", cfg.Plan)
	}
	// Defaults apply when the inline plan omits the altitude/fraction fields.
	if cfg.Plan.CruiseAltitudeM != 10668 || cfg.Plan.ClimbFraction != 0.25 {
		t.Errorf("defaults not applied: %+v", cfg.Plan)
	}
}

func TestLoad_RequiresAPlan(t *testing.T) {
	// Neither FLIGHT_PLAN_JSON nor FLIGHT_PLAN set.
	if _, err := Load(); err == nil {
		t.Fatal("expected an error when no plan is provided")
	}
}

func TestLoad_OperationsRequiresInstanceID(t *testing.T) {
	t.Setenv("FLIGHT_PLAN_JSON", inlinePlan)
	t.Setenv("OPERATIONS_URL", "http://operations:8080")
	// FLIGHT_INSTANCE_ID deliberately unset.

	if _, err := Load(); err == nil {
		t.Fatal("expected an error: OPERATIONS_URL without FLIGHT_INSTANCE_ID")
	}

	t.Setenv("FLIGHT_INSTANCE_ID", "run-1-0000")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with instance id: %v", err)
	}
	if cfg.OperationsURL == "" || cfg.FlightInstanceID != "run-1-0000" {
		t.Errorf("operations config not resolved: %+v", cfg)
	}
}
