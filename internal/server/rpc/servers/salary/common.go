package salary

import "github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"

func validateYearMonth(year, month int32) error {
	if year < 2000 || year > 9999 || month < 1 || month > 12 {
		return viber.InvalidArgumentf("invalid salary year/month")
	}
	return nil
}
