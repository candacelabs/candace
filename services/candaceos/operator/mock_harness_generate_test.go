package operator_test

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_runtime_test.go -package=operator_test github.com/candacelabs/candace/services/candaceos/harness Runtime
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_reconciler_test.go -package=operator_test github.com/candacelabs/candace/services/candaceos/operator Reconciler
