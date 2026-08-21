package viber

import (
	cryptorand "crypto/rand"
	"math/big"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func CryptoRandomIndex(length int) (int, error) {
	if length <= 0 {
		return 0, status.Error(codes.FailedPrecondition, "cannot select from an empty catalog")
	}
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(length)))
	if err != nil {
		return 0, status.Errorf(codes.Internal, "generate secure random selection: %v", err)
	}
	return int(value.Int64()), nil
}
