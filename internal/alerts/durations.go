package alerts

// CreateRuleDurations resolves API omission separately from an explicit zero.
// Interval/window keep their existing zero-to-default compatibility.
type CreateRuleDurations struct {
	Cooldown, Reopen, Interval, Window int
}

func ResolveCreateRuleDurations(req CreateRuleRequest) (CreateRuleDurations, error) {
	out := CreateRuleDurations{}
	for _, field := range []struct {
		input       *int
		fallback    int
		zeroDefault bool
		output      *int
	}{
		{req.CooldownSeconds, DefaultCooldownSeconds, false, &out.Cooldown},
		{req.ReopenAfterSeconds, DefaultReopenAfterSeconds, false, &out.Reopen},
		{req.EvaluationIntervalSeconds, DefaultEvaluationIntervalSeconds, true, &out.Interval},
		{req.WindowSeconds, DefaultWindowSeconds, true, &out.Window},
	} {
		value := field.fallback
		if field.input != nil {
			value = *field.input
			if value < 0 || value > MaxDurationSeconds {
				return CreateRuleDurations{}, ErrInvalidDurationSeconds
			}
			if value == 0 && field.zeroDefault {
				value = field.fallback
			}
		}
		*field.output = value
	}
	return out, nil
}
