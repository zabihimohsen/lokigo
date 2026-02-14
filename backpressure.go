package lokigo

import (
	"context"
	"errors"
)

var errDroppedInternal = errors.New("dropped")

func enqueueWithMode(ctx context.Context, ch chan Entry, v Entry, mode BackpressureMode) error {
	switch mode {
	case BackpressureBlock:
		select {
		case ch <- v:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	case BackpressureDropNew:
		select {
		case ch <- v:
			return nil
		default:
			return errDroppedInternal
		}
	case BackpressureDropOldest:
		for {
			select {
			case ch <- v:
				return nil
			default:
				select {
				case <-ch:
				default:
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
	default:
		return errors.New("unknown backpressure mode")
	}
}
