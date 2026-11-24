package main

// eMudScan's own connection to look for the latest release of what a fix downloads
// (latest.go): Windows' HTTPS, as for the online test, with at most 8 seconds for each
// step of a connection. An answer that is not a success, or that fills all the room
// given (it may be cut), is not used.
func init() {
	httpsGetWatched = func(host, path string, max int, watch func(got int) bool) ([]byte, bool) {
		r := winhttpFetch(host, path, 8000, true, max, watch)
		if !r.Connected || r.Stopped || r.Status != 200 || len(r.Body) >= max {
			return nil, false
		}
		return []byte(r.Body), true
	}
	httpsGet = func(host, path string, max int) ([]byte, bool) { return httpsGetWatched(host, path, max, nil) }
}
