package service

import "math"

func normalizePageLimit(offset, limit int) (int, error) {
	if limit == 0 {
		limit = 20
	}
	if offset < 0 || limit < 1 || limit > 100 || offset > math.MaxInt-limit {
		return 0, ErrInvalidInput
	}
	return limit, nil
}
