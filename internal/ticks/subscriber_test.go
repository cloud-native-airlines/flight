package ticks

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/cloud-native-airlines/flight/internal/engine"
	"github.com/cloud-native-airlines/flight/internal/flight"
)

type emitCall struct {
	runID, eventType, aircraftID string
	at                           time.Time
}

type fakeEmitter struct{ calls []emitCall }

func (f *fakeEmitter) Emit(_ context.Context, runID, eventType, aircraftID string, at time.Time) error {
	f.calls = append(f.calls, emitCall{runID, eventType, aircraftID, at})
	return nil
}

func newTestSubscriber(e EventEmitter) *Subscriber {
	return &Subscriber{emitter: e, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func flyingResult(at time.Time) engine.Result {
	return engine.Result{State: flight.State{SimulatedAt: at, Status: flight.StatusFlying}, Applied: true}
}

func arrivedResult(at time.Time) engine.Result {
	return engine.Result{
		State:    flight.State{SimulatedAt: at, Status: flight.StatusParked, Phase: flight.PhaseArrived},
		Applied:  true,
		Terminal: true,
	}
}

func TestEmitLifecycle_DepartureOnceThenArrival(t *testing.T) {
	f := &fakeEmitter{}
	s := newTestSubscriber(f)

	t0 := time.Date(2026, 1, 1, 12, 5, 0, 0, time.UTC)
	s.emitLifecycle("run-1", flyingResult(t0), "N100CA")
	s.emitLifecycle("run-1", flyingResult(t0.Add(time.Minute)), "N100CA") // still flying
	s.emitLifecycle("run-1", arrivedResult(t0.Add(40*time.Minute)), "N100CA")

	if len(f.calls) != 2 {
		t.Fatalf("want 2 events (one departure, one arrival), got %d: %+v", len(f.calls), f.calls)
	}
	if f.calls[0].eventType != "departure" || !f.calls[0].at.Equal(t0) {
		t.Errorf("first event should be departure at %v, got %+v", t0, f.calls[0])
	}
	if f.calls[1].eventType != "arrival" {
		t.Errorf("second event should be arrival, got %+v", f.calls[1])
	}
}

func TestEmitLifecycle_ArrivalWithoutPriorFlyingStillEmitsDeparture(t *testing.T) {
	f := &fakeEmitter{}
	s := newTestSubscriber(f)

	at := time.Date(2026, 1, 1, 12, 40, 0, 0, time.UTC)
	s.emitLifecycle("run-1", arrivedResult(at), "N100CA")

	if len(f.calls) != 2 {
		t.Fatalf("want 2 events (departure then arrival), got %d: %+v", len(f.calls), f.calls)
	}
	if f.calls[0].eventType != "departure" || f.calls[1].eventType != "arrival" {
		t.Errorf("want [departure, arrival], got [%s, %s]", f.calls[0].eventType, f.calls[1].eventType)
	}
}
