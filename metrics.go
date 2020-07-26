package main

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricLastDeviceLocationTime = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "last_device_location_at",
		Help: "Unix timestamp when of the last device location reported",
	})
	metricLatestFoursquareCheckin = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "last_foursquare_checkin_at",
		Help: "Unix time timestamp when the last foursquare checkin was recorded into the DB",
	})

	metricOTSubmitSuccessCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "owntracks_publish_endpoint_success",
		Help: "Number of successful requests served at the OwnTracks publishing endpoint",
	})
	metricOTSubmitErrorCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "owntracks_publish_endpoint_errors",
		Help: "Number of errors served at the OwnTracks publishing endpoint",
	})

	metric4sqSyncSuccessCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "foursquare_sync_success_count",
		Help: "Number of successful syncs with foursquare",
	})
	metric4sqSyncErrorCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "foursquare_sync_error_count",
		Help: "Number of syncs that failed with foursquare",
	})

	metricTripitSyncSuccessCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tripit_sync_success_count",
		Help: "Number of successful syncs with TripIt",
	})
	metricTripitSyncErrorCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tripit_sync_error_count",
		Help: "Number of syncs that failed with TripIt",
	})
)

// collectProcessMetrics runs a background task to populate various metrics
// about the running app. this should be run regularly
func collectProcessMetrics(ctx context.Context, s *Storage) error {
	lt, err := s.LatestLocationTimestamp(ctx)
	if err != nil {
		return fmt.Errorf("checking latest device location time: %v", err)
	}
	metricLastDeviceLocationTime.Set(float64(lt.Unix()))

	lfsq, err := s.Last4sqCheckinTime(ctx)
	if err != nil {
		return fmt.Errorf("checking latest foursquare checkin time: %v", err)
	}
	metricLatestFoursquareCheckin.Set(float64(lfsq.Unix()))

	return nil
}
