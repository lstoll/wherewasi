package main

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
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

var _ prometheus.Collector = (*metricsCollector)(nil)

type metricsCollector struct {
	l logger
	s *Storage

	lastDeviceLocationTime  *prometheus.Desc
	latestFoursquareCheckin *prometheus.Desc
}

func newMetricsCollector(l logger, s *Storage) *metricsCollector {
	return &metricsCollector{
		l: l,
		s: s,

		lastDeviceLocationTime: prometheus.NewDesc(
			"last_device_location_at",
			"Unix timestamp when of the last device location reported", nil, nil),
		latestFoursquareCheckin: prometheus.NewDesc(
			"last_foursquare_checkin_at",
			"Unix time timestamp when the last foursquare checkin was recorded into the DB", nil, nil),
	}
}

func (m *metricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- m.lastDeviceLocationTime
	ch <- m.latestFoursquareCheckin
}

func (m *metricsCollector) Collect(ch chan<- prometheus.Metric) {
	// we don't get a context from the scrape here, so just create one with a
	// reasonably timeout to avoid blocking things here
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lt, err := m.s.LatestLocationTimestamp(ctx)
	if err == nil {
		ch <- prometheus.MustNewConstMetric(m.lastDeviceLocationTime, prometheus.GaugeValue, float64(lt.Unix()))
	} else {
		ch <- prometheus.NewInvalidMetric(m.lastDeviceLocationTime, fmt.Errorf("get latest device location timestamp: %v", err))
	}

	lfsq, err := m.s.Last4sqCheckinTime(ctx)
	if err == nil {
		ch <- prometheus.MustNewConstMetric(m.latestFoursquareCheckin, prometheus.GaugeValue, float64(lfsq.Unix()))
	} else {
		ch <- prometheus.NewInvalidMetric(m.latestFoursquareCheckin, fmt.Errorf("get latest foursquare checkin timestamp: %v", err))
	}
}
