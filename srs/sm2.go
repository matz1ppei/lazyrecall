package srs

import "math"

type CardState struct {
	Interval    int
	EaseFactor  float64
	Repetitions int
}

func NextState(s CardState, rating int) CardState {
	if rating < 3 {
		return CardState{
			Interval:    1,
			EaseFactor:  math.Max(s.EaseFactor, 1.3),
			Repetitions: 0,
		}
	}

	var interval int
	switch s.Repetitions {
	case 0:
		interval = 1
	case 1:
		interval = 6
	default:
		interval = int(math.Round(float64(s.Interval) * s.EaseFactor))
	}

	diff := float64(5 - rating)
	ef := s.EaseFactor + (0.1 - diff*(0.08+diff*0.02))
	ef = math.Max(ef, 1.3)

	return CardState{
		Interval:    interval,
		EaseFactor:  ef,
		Repetitions: s.Repetitions + 1,
	}
}
