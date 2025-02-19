package tariff

import (
	"errors"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/util/config"
	"github.com/evcc-io/evcc/util/request"
)

// Name returns the tariff type name
func Name(conf config.Typed) string {
	if conf.Other != nil && conf.Other["tariff"] != nil {
		return conf.Other["tariff"].(string)
	}
	return conf.Type
}

func bo() backoff.BackOff {
	return backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(time.Second),
		backoff.WithMaxElapsedTime(time.Minute),
	)
}

// backoffPermanentError returns a permanent error in case of HTTP 400
func backoffPermanentError(err error) error {
	if se := new(request.StatusError); errors.As(err, &se) {
		if code := se.StatusCode(); code >= 400 && code <= 599 {
			return backoff.Permanent(se)
		}
	}
	if err != nil && strings.HasPrefix(err.Error(), "jq: query failed") {
		return backoff.Permanent(err)
	}
	return err
}

// mergeRates merges new rates into existing rates,
// keeping current slots from the existing rates.
func mergeRates(data *util.Monitor[api.Rates], new api.Rates) {
	new.Sort()

	var newStart time.Time
	if len(new) > 0 {
		newStart = new[0].Start
	}

	data.SetFunc(func(old api.Rates) api.Rates {
		now := time.Now()

		var between api.Rates
		for _, r := range old {
			if (r.Start.Before(newStart) || newStart.IsZero()) && r.End.After(now) {
				between = append(between, r)
			}
		}

		return append(between, new...)
	})
}
