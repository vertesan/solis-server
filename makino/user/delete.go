package user

import (
	"context"
	"vertesan/solis-server/db/crud/opuser"
	"vertesan/solis-server/db/transaction"
)

func DeleteUser(suid string) error {
	err := transaction.WithTransaction(func(ctx context.Context) (interface{}, error) {
		if err := opuser.DeleteUserAuth(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteUser(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserItems(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserCards(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserCharacters(ctx, suid); err != nil {
			return nil, err
		}
		if err := opuser.DeleteAllUserCostumes(ctx, suid); err != nil {
			return nil, err
		}
		
		return nil, nil
	}, nil, nil)
	if err != nil {
		return err
	}
	return nil
}
