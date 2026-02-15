package lokigo

import (
	"context"
	"errors"
)

var errDroppedInternal = errors.New("dropped")

func enqueueWithMode(ctx context.Context, ch chan Entry, v Entry, mode BackpressureMode) (int, error) {
	switch mode {
	case BackpressureBlock:
		select {
		case ch <- v:
			return 0, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	case BackpressureDropNew:
		select {
		case ch <- v:
			return 0, nil
		default:
			return 1, errDroppedInternal
		}
	case BackpressureDropOldest:
		dropped := 0
		for {
			select {
			case ch <- v:
				return dropped, nil
			default:
				select {
				case <-ch:
					dropped++
				default:
				}
			}
			select {
			case <-ctx.Done():
				return dropped, ctx.Err()
			default:
			}
		}
	default:
		return 0, errors.New("unknown backpressure mode")
	}
}
