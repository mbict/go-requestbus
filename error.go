package requestbus

type BusError struct {
	error string

	RequestName string
	ResultType  string
}

func (r BusError) Error() string {
	return r.error
}
