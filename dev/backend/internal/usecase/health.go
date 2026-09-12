package usecase

import "context"

// ReadinessChecker は稼働準備を確認する。
type ReadinessChecker interface {
	Ping(context.Context) error
}

type ReadinessUseCase struct {
	checker ReadinessChecker
}

func NewReadinessUseCase(checker ReadinessChecker) ReadinessUseCase {
	return ReadinessUseCase{checker: checker}
}

func (u ReadinessUseCase) Execute(ctx context.Context) error {
	return u.checker.Ping(ctx)
}
