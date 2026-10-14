package repository

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/medxops/trazr-otel/pkg/event"
)

// Filters for metric streaming (expand as needed)
type MetricStreamFilter struct {
	CommonBaseFilters
	CommonAdvancedFilters

	// Metric-specific streaming filters
	MetricName string
}

type GaugeMetricStreamRepository interface {
	SubscribeGaugeMetrics(ctx context.Context, filter MetricStreamFilter) (<-chan GaugeMetricEntry, func(), <-chan struct{}, error)
}

type eventBusGaugeMetricStreamRepository struct {
	bus         event.EventBusInterface
	subscribeMu sync.Mutex
	subscribers map[chan GaugeMetricEntry]struct{}
}

func NewGaugeMetricStreamRepository(bus event.EventBusInterface) GaugeMetricStreamRepository {
	return &eventBusGaugeMetricStreamRepository{
		bus:         bus,
		subscribers: make(map[chan GaugeMetricEntry]struct{}),
	}
}

// Alias for extension compatibility
func NewGaugeMetricStream(bus event.EventBusInterface) GaugeMetricStreamRepository {
	return NewGaugeMetricStreamRepository(bus)
}

func (r *eventBusGaugeMetricStreamRepository) SubscribeGaugeMetrics(ctx context.Context, filter MetricStreamFilter) (<-chan GaugeMetricEntry, func(), <-chan struct{}, error) {
	msgCh := make(chan GaugeMetricEntry, streamBufferSize)
	done := make(chan struct{})
	established := make(chan struct{})

	handler := &gaugeMetricEventHandler{
		filter: filter,
		msgCh:  msgCh,
		done:   done,
	}

	r.subscribeMu.Lock()
	r.subscribers[msgCh] = struct{}{}
	r.subscribeMu.Unlock()

	subscription := r.bus.Subscribe(EventTypeGaugeMetricInserted.ToEventType(), handler)
	close(established)

	cancel := func() {
		close(done)
		if subscription != nil {
			_ = subscription.Close()
		}
		r.subscribeMu.Lock()
		delete(r.subscribers, msgCh)
		r.subscribeMu.Unlock()
	}

	return msgCh, cancel, established, nil
}

type SumMetricStreamRepository interface {
	SubscribeSumMetrics(ctx context.Context, filter MetricStreamFilter) (<-chan SumMetricEntry, func(), <-chan struct{}, error)
}

type eventBusSumMetricStreamRepository struct {
	bus         event.EventBusInterface
	subscribeMu sync.Mutex
	subscribers map[chan SumMetricEntry]struct{}
}

func NewSumMetricStreamRepository(bus event.EventBusInterface) SumMetricStreamRepository {
	return &eventBusSumMetricStreamRepository{
		bus:         bus,
		subscribers: make(map[chan SumMetricEntry]struct{}),
	}
}

// Alias for extension compatibility
func NewSumMetricStream(bus event.EventBusInterface) SumMetricStreamRepository {
	return NewSumMetricStreamRepository(bus)
}

func (r *eventBusSumMetricStreamRepository) SubscribeSumMetrics(ctx context.Context, filter MetricStreamFilter) (<-chan SumMetricEntry, func(), <-chan struct{}, error) {
	msgCh := make(chan SumMetricEntry, streamBufferSize)
	done := make(chan struct{})
	established := make(chan struct{})

	handler := &sumMetricEventHandler{
		filter: filter,
		msgCh:  msgCh,
		done:   done,
	}

	r.subscribeMu.Lock()
	r.subscribers[msgCh] = struct{}{}
	r.subscribeMu.Unlock()

	subscription := r.bus.Subscribe(EventTypeSumMetricInserted.ToEventType(), handler)
	close(established)

	cancel := func() {
		close(done)
		if subscription != nil {
			_ = subscription.Close()
		}
		r.subscribeMu.Lock()
		delete(r.subscribers, msgCh)
		r.subscribeMu.Unlock()
	}

	return msgCh, cancel, established, nil
}

type HistogramMetricStreamRepository interface {
	SubscribeHistogramMetrics(ctx context.Context, filter MetricStreamFilter) (<-chan HistogramMetricEntry, func(), <-chan struct{}, error)
}

type eventBusHistogramMetricStreamRepository struct {
	bus         event.EventBusInterface
	subscribeMu sync.Mutex
	subscribers map[chan HistogramMetricEntry]struct{}
}

func NewHistogramMetricStreamRepository(bus event.EventBusInterface) HistogramMetricStreamRepository {
	return &eventBusHistogramMetricStreamRepository{
		bus:         bus,
		subscribers: make(map[chan HistogramMetricEntry]struct{}),
	}
}

// Alias for extension compatibility
func NewHistogramMetricStream(bus event.EventBusInterface) HistogramMetricStreamRepository {
	return NewHistogramMetricStreamRepository(bus)
}

func (r *eventBusHistogramMetricStreamRepository) SubscribeHistogramMetrics(ctx context.Context, filter MetricStreamFilter) (<-chan HistogramMetricEntry, func(), <-chan struct{}, error) {
	msgCh := make(chan HistogramMetricEntry, streamBufferSize)
	done := make(chan struct{})
	established := make(chan struct{})

	handler := &histogramMetricEventHandler{
		filter: filter,
		msgCh:  msgCh,
		done:   done,
	}

	r.subscribeMu.Lock()
	r.subscribers[msgCh] = struct{}{}
	r.subscribeMu.Unlock()

	subscription := r.bus.Subscribe(EventTypeHistogramMetricInserted.ToEventType(), handler)
	close(established)

	cancel := func() {
		close(done)
		if subscription != nil {
			_ = subscription.Close()
		}
		r.subscribeMu.Lock()
		delete(r.subscribers, msgCh)
		r.subscribeMu.Unlock()
	}

	return msgCh, cancel, established, nil
}

type SummaryMetricStreamRepository interface {
	SubscribeSummaryMetrics(ctx context.Context, filter MetricStreamFilter) (<-chan SummaryMetricEntry, func(), <-chan struct{}, error)
}

type eventBusSummaryMetricStreamRepository struct {
	bus         event.EventBusInterface
	subscribeMu sync.Mutex
	subscribers map[chan SummaryMetricEntry]struct{}
}

func NewSummaryMetricStreamRepository(bus event.EventBusInterface) SummaryMetricStreamRepository {
	return &eventBusSummaryMetricStreamRepository{
		bus:         bus,
		subscribers: make(map[chan SummaryMetricEntry]struct{}),
	}
}

// Alias for extension compatibility
func NewSummaryMetricStream(bus event.EventBusInterface) SummaryMetricStreamRepository {
	return NewSummaryMetricStreamRepository(bus)
}

func (r *eventBusSummaryMetricStreamRepository) SubscribeSummaryMetrics(ctx context.Context, filter MetricStreamFilter) (<-chan SummaryMetricEntry, func(), <-chan struct{}, error) {
	msgCh := make(chan SummaryMetricEntry, streamBufferSize)
	done := make(chan struct{})
	established := make(chan struct{})

	handler := &summaryMetricEventHandler{
		filter: filter,
		msgCh:  msgCh,
		done:   done,
	}

	r.subscribeMu.Lock()
	r.subscribers[msgCh] = struct{}{}
	r.subscribeMu.Unlock()

	subscription := r.bus.Subscribe(EventTypeSummaryMetricInserted.ToEventType(), handler)
	close(established)

	cancel := func() {
		close(done)
		if subscription != nil {
			_ = subscription.Close()
		}
		r.subscribeMu.Lock()
		delete(r.subscribers, msgCh)
		r.subscribeMu.Unlock()
	}

	return msgCh, cancel, established, nil
}

type ExponentialHistogramMetricStreamRepository interface {
	SubscribeExponentialHistogramMetrics(ctx context.Context, filter MetricStreamFilter) (<-chan ExponentialHistogramMetricEntry, func(), <-chan struct{}, error)
}

type eventBusExponentialHistogramMetricStreamRepository struct {
	bus         event.EventBusInterface
	subscribeMu sync.Mutex
	subscribers map[chan ExponentialHistogramMetricEntry]struct{}
}

func NewExponentialHistogramMetricStreamRepository(bus event.EventBusInterface) ExponentialHistogramMetricStreamRepository {
	return &eventBusExponentialHistogramMetricStreamRepository{
		bus:         bus,
		subscribers: make(map[chan ExponentialHistogramMetricEntry]struct{}),
	}
}

// Alias for extension compatibility
func NewExponentialHistogramMetricStream(bus event.EventBusInterface) ExponentialHistogramMetricStreamRepository {
	return NewExponentialHistogramMetricStreamRepository(bus)
}

func (r *eventBusExponentialHistogramMetricStreamRepository) SubscribeExponentialHistogramMetrics(ctx context.Context, filter MetricStreamFilter) (<-chan ExponentialHistogramMetricEntry, func(), <-chan struct{}, error) {
	msgCh := make(chan ExponentialHistogramMetricEntry, streamBufferSize)
	done := make(chan struct{})
	established := make(chan struct{})

	handler := &exponentialHistogramMetricEventHandler{
		filter: filter,
		msgCh:  msgCh,
		done:   done,
	}

	r.subscribeMu.Lock()
	r.subscribers[msgCh] = struct{}{}
	r.subscribeMu.Unlock()

	subscription := r.bus.Subscribe(EventTypeExponentialHistogramMetricInserted.ToEventType(), handler)
	close(established)

	cancel := func() {
		close(done)
		if subscription != nil {
			_ = subscription.Close()
		}
		r.subscribeMu.Lock()
		delete(r.subscribers, msgCh)
		r.subscribeMu.Unlock()
	}

	return msgCh, cancel, established, nil
}

type gaugeMetricEventHandler struct {
	filter MetricStreamFilter
	msgCh  chan<- GaugeMetricEntry
	done   <-chan struct{}
}

func (h *gaugeMetricEventHandler) HandleEvent(ctx context.Context, ev event.Event) error {
	select {
	case <-h.done:
		return nil
	default:
	}

	// Check schema filter first - if schema doesn't match, skip processing
	if h.filter.Schema != "" && ev.Schema != h.filter.Schema {
		return nil
	}

	if single, ok := ev.Data.(GaugeMetricEntry); ok {
		if h.matchesFilter(single) {
			select {
			case <-h.done:
				return nil
			case h.msgCh <- single:
			}
		}
		return nil
	}
	if entries, ok := ev.Data.([]GaugeMetricEntry); ok {
		for _, e := range entries {
			if h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}

	if m, ok := ev.Data.(map[string]interface{}); ok {
		b, err := json.Marshal(m)
		if err == nil {
			var e GaugeMetricEntry
			if json.Unmarshal(b, &e) == nil && h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	if rawSlice, ok := ev.Data.([]interface{}); ok {
		for _, raw := range rawSlice {
			b, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var e GaugeMetricEntry
			if json.Unmarshal(b, &e) != nil {
				continue
			}
			if h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	return nil
}

func (h *gaugeMetricEventHandler) matchesFilter(e GaugeMetricEntry) bool {
	// Create FilterMatcher for consistent filtering
	matcher := &FilterMatcher{}

	// Check common base filters
	if !matcher.MatchesCommonBaseFilters(h.filter.CommonBaseFilters, e.ServiceName) {
		return false
	}

	// Extract attributes from JSONB fields
	attributes := matcher.ExtractAttributesFromJSON(e.Attributes)
	resourceAttributes := matcher.ExtractAttributesFromJSON(e.ResourceAttributes)

	// Use metric name as the "body" text for text search
	metricNameText := ""
	if e.Name != nil {
		metricNameText = *e.Name
	}

	// Check common advanced filters (attributes, text search)
	if !matcher.MatchesCommonAdvancedFilters(h.filter.CommonAdvancedFilters, attributes, resourceAttributes, metricNameText) {
		return false
	}

	// Metric-specific filters
	if h.filter.MetricName != "" && (e.Name == nil || !strings.Contains(strings.ToLower(*e.Name), strings.ToLower(h.filter.MetricName))) {
		return false
	}
	// Schema filtering is handled at the event level in HandleEvent
	return true
}

type sumMetricEventHandler struct {
	filter MetricStreamFilter
	msgCh  chan<- SumMetricEntry
	done   <-chan struct{}
}

func (h *sumMetricEventHandler) HandleEvent(ctx context.Context, ev event.Event) error {
	select {
	case <-h.done:
		return nil
	default:
	}

	// Check schema filter first - if schema doesn't match, skip processing
	if h.filter.Schema != "" && ev.Schema != h.filter.Schema {
		return nil
	}

	if single, ok := ev.Data.(SumMetricEntry); ok {
		if h.matchesFilter(single) {
			select {
			case <-h.done:
				return nil
			case h.msgCh <- single:
			}
		}
		return nil
	}
	if slice, ok := ev.Data.([]SumMetricEntry); ok {
		for _, e := range slice {
			if h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	if m, ok := ev.Data.(map[string]interface{}); ok {
		b, err := json.Marshal(m)
		if err == nil {
			var e SumMetricEntry
			if json.Unmarshal(b, &e) == nil && h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	if rawSlice, ok := ev.Data.([]interface{}); ok {
		for _, raw := range rawSlice {
			b, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var e SumMetricEntry
			if json.Unmarshal(b, &e) != nil {
				continue
			}
			if h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	return nil
}

func (h *sumMetricEventHandler) matchesFilter(e SumMetricEntry) bool {
	// Create FilterMatcher for consistent filtering
	matcher := &FilterMatcher{}

	// Check common base filters
	if !matcher.MatchesCommonBaseFilters(h.filter.CommonBaseFilters, e.ServiceName) {
		return false
	}

	// Extract attributes from JSONB fields
	attributes := matcher.ExtractAttributesFromJSON(e.Attributes)
	resourceAttributes := matcher.ExtractAttributesFromJSON(e.ResourceAttributes)

	// Use metric name as the "body" text for text search
	metricNameText := ""
	if e.Name != nil {
		metricNameText = *e.Name
	}

	// Check common advanced filters (attributes, text search)
	if !matcher.MatchesCommonAdvancedFilters(h.filter.CommonAdvancedFilters, attributes, resourceAttributes, metricNameText) {
		return false
	}

	// Metric-specific filters
	if h.filter.MetricName != "" && (e.Name == nil || !strings.Contains(strings.ToLower(*e.Name), strings.ToLower(h.filter.MetricName))) {
		return false
	}
	// Schema filtering is handled at the event level in HandleEvent
	return true
}

type histogramMetricEventHandler struct {
	filter MetricStreamFilter
	msgCh  chan<- HistogramMetricEntry
	done   <-chan struct{}
}

func (h *histogramMetricEventHandler) HandleEvent(ctx context.Context, ev event.Event) error {
	select {
	case <-h.done:
		return nil
	default:
	}

	// Check schema filter first - if schema doesn't match, skip processing
	if h.filter.Schema != "" && ev.Schema != h.filter.Schema {
		return nil
	}

	if single, ok := ev.Data.(HistogramMetricEntry); ok {
		if h.matchesFilter(single) {
			select {
			case <-h.done:
				return nil
			case h.msgCh <- single:
			}
		}
		return nil
	}
	if slice, ok := ev.Data.([]HistogramMetricEntry); ok {
		for _, e := range slice {
			if h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	if m, ok := ev.Data.(map[string]interface{}); ok {
		b, err := json.Marshal(m)
		if err == nil {
			var e HistogramMetricEntry
			if json.Unmarshal(b, &e) == nil && h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	if rawSlice, ok := ev.Data.([]interface{}); ok {
		for _, raw := range rawSlice {
			b, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var e HistogramMetricEntry
			if json.Unmarshal(b, &e) != nil {
				continue
			}
			if h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	return nil
}

func (h *histogramMetricEventHandler) matchesFilter(e HistogramMetricEntry) bool {
	// Create FilterMatcher for consistent filtering
	matcher := &FilterMatcher{}

	// Check common base filters
	if !matcher.MatchesCommonBaseFilters(h.filter.CommonBaseFilters, e.ServiceName) {
		return false
	}

	// Extract attributes from JSONB fields
	attributes := matcher.ExtractAttributesFromJSON(e.Attributes)
	resourceAttributes := matcher.ExtractAttributesFromJSON(e.ResourceAttributes)

	// Use metric name as the "body" text for text search
	metricNameText := ""
	if e.Name != nil {
		metricNameText = *e.Name
	}

	// Check common advanced filters (attributes, text search)
	if !matcher.MatchesCommonAdvancedFilters(h.filter.CommonAdvancedFilters, attributes, resourceAttributes, metricNameText) {
		return false
	}

	// Metric-specific filters
	if h.filter.MetricName != "" && (e.Name == nil || !strings.Contains(strings.ToLower(*e.Name), strings.ToLower(h.filter.MetricName))) {
		return false
	}
	// Schema filtering is handled at the event level in HandleEvent
	return true
}

type summaryMetricEventHandler struct {
	filter MetricStreamFilter
	msgCh  chan<- SummaryMetricEntry
	done   <-chan struct{}
}

func (h *summaryMetricEventHandler) HandleEvent(ctx context.Context, ev event.Event) error {
	select {
	case <-h.done:
		return nil
	default:
	}

	// Check schema filter first - if schema doesn't match, skip processing
	if h.filter.Schema != "" && ev.Schema != h.filter.Schema {
		return nil
	}

	if single, ok := ev.Data.(SummaryMetricEntry); ok {
		if h.matchesFilter(single) {
			select {
			case <-h.done:
				return nil
			case h.msgCh <- single:
			}
		}
		return nil
	}
	if slice, ok := ev.Data.([]SummaryMetricEntry); ok {
		for _, e := range slice {
			if h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	if m, ok := ev.Data.(map[string]interface{}); ok {
		b, err := json.Marshal(m)
		if err == nil {
			var e SummaryMetricEntry
			if json.Unmarshal(b, &e) == nil && h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	if rawSlice, ok := ev.Data.([]interface{}); ok {
		for _, raw := range rawSlice {
			b, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var e SummaryMetricEntry
			if json.Unmarshal(b, &e) != nil {
				continue
			}
			if h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	return nil
}

func (h *summaryMetricEventHandler) matchesFilter(e SummaryMetricEntry) bool {
	// Create FilterMatcher for consistent filtering
	matcher := &FilterMatcher{}

	// Check common base filters
	if !matcher.MatchesCommonBaseFilters(h.filter.CommonBaseFilters, e.ServiceName) {
		return false
	}

	// Extract attributes from JSONB fields
	attributes := matcher.ExtractAttributesFromJSON(e.Attributes)
	resourceAttributes := matcher.ExtractAttributesFromJSON(e.ResourceAttributes)

	// Use metric name as the "body" text for text search
	metricNameText := ""
	if e.Name != nil {
		metricNameText = *e.Name
	}

	// Check common advanced filters (attributes, text search)
	if !matcher.MatchesCommonAdvancedFilters(h.filter.CommonAdvancedFilters, attributes, resourceAttributes, metricNameText) {
		return false
	}

	// Metric-specific filters
	if h.filter.MetricName != "" && (e.Name == nil || !strings.Contains(strings.ToLower(*e.Name), strings.ToLower(h.filter.MetricName))) {
		return false
	}
	// Schema filtering is handled at the event level in HandleEvent
	return true
}

type exponentialHistogramMetricEventHandler struct {
	filter MetricStreamFilter
	msgCh  chan<- ExponentialHistogramMetricEntry
	done   <-chan struct{}
}

func (h *exponentialHistogramMetricEventHandler) HandleEvent(ctx context.Context, ev event.Event) error {
	select {
	case <-h.done:
		return nil
	default:
	}

	// Check schema filter first - if schema doesn't match, skip processing
	if h.filter.Schema != "" && ev.Schema != h.filter.Schema {
		return nil
	}

	if single, ok := ev.Data.(ExponentialHistogramMetricEntry); ok {
		if h.matchesFilter(single) {
			select {
			case <-h.done:
				return nil
			case h.msgCh <- single:
			}
		}
		return nil
	}
	if slice, ok := ev.Data.([]ExponentialHistogramMetricEntry); ok {
		for _, e := range slice {
			if h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	if m, ok := ev.Data.(map[string]interface{}); ok {
		b, err := json.Marshal(m)
		if err == nil {
			var e ExponentialHistogramMetricEntry
			if json.Unmarshal(b, &e) == nil && h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	if rawSlice, ok := ev.Data.([]interface{}); ok {
		for _, raw := range rawSlice {
			b, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var e ExponentialHistogramMetricEntry
			if json.Unmarshal(b, &e) != nil {
				continue
			}
			if h.matchesFilter(e) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- e:
				}
			}
		}
		return nil
	}
	return nil
}

func (h *exponentialHistogramMetricEventHandler) matchesFilter(e ExponentialHistogramMetricEntry) bool {
	// Create FilterMatcher for consistent filtering
	matcher := &FilterMatcher{}

	// Check common base filters
	if !matcher.MatchesCommonBaseFilters(h.filter.CommonBaseFilters, e.ServiceName) {
		return false
	}

	// Extract attributes from JSONB fields
	attributes := matcher.ExtractAttributesFromJSON(e.Attributes)
	resourceAttributes := matcher.ExtractAttributesFromJSON(e.ResourceAttributes)

	// Use metric name as the "body" text for text search
	metricNameText := ""
	if e.Name != nil {
		metricNameText = *e.Name
	}

	// Check common advanced filters (attributes, text search)
	if !matcher.MatchesCommonAdvancedFilters(h.filter.CommonAdvancedFilters, attributes, resourceAttributes, metricNameText) {
		return false
	}

	// Metric-specific filters
	if h.filter.MetricName != "" && (e.Name == nil || !strings.Contains(strings.ToLower(*e.Name), strings.ToLower(h.filter.MetricName))) {
		return false
	}
	// Schema filtering is handled at the event level in HandleEvent
	return true
}
