package keeper

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/public-awesome/stargaze/x/alloc/types"
)

type (
	Keeper struct {
		cdc      codec.BinaryCodec
		storeKey sdk.StoreKey
		memKey   sdk.StoreKey

		accountKeeper types.AccountKeeper
		bankKeeper    types.BankKeeper
		stakingKeeper types.StakingKeeper
		distrKeeper   types.DistrKeeper

		paramstore paramtypes.Subspace
	}
)

func NewKeeper(
	cdc codec.BinaryCodec,
	storeKey,
	memKey sdk.StoreKey,

	accountKeeper types.AccountKeeper, bankKeeper types.BankKeeper, stakingKeeper types.StakingKeeper, distrKeeper types.DistrKeeper,
	ps paramtypes.Subspace,
) *Keeper {

	// set KeyTable if it has not already been set
	if !ps.HasKeyTable() {
		ps = ps.WithKeyTable(types.ParamKeyTable())
	}

	return &Keeper{
		cdc:      cdc,
		storeKey: storeKey,
		memKey:   memKey,

		accountKeeper: accountKeeper, bankKeeper: bankKeeper, stakingKeeper: stakingKeeper, distrKeeper: distrKeeper,
		paramstore: ps,
	}
}

func (k Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", fmt.Sprintf("x/%s", types.ModuleName))
}

// GetModuleAccountBalance gets the airdrop coin balance of module account
func (k Keeper) GetModuleAccountAddress(ctx sdk.Context) sdk.AccAddress {
	return k.accountKeeper.GetModuleAddress(types.ModuleName)
}

// DistributeInflation distributes module-specific inflation
func (k Keeper) DistributeInflation(ctx sdk.Context) error {
	blockInflationAddr := k.accountKeeper.GetModuleAccount(ctx, authtypes.FeeCollectorName).GetAddress()
	blockInflation := k.bankKeeper.GetBalance(ctx, blockInflationAddr, k.stakingKeeper.BondDenom(ctx))
	blockInflationDec := sdk.NewDecFromInt(blockInflation.Amount)

	params := k.GetParams(ctx)
	proportions := params.DistributionProportions

	daoRewardAmount := blockInflationDec.Mul(proportions.DaoRewards)
	daoRewardCoin := sdk.NewCoin(k.stakingKeeper.BondDenom(ctx), daoRewardAmount.TruncateInt())
	// Distribute DAO incentives to the community pool until StargazeDAO is implemented
	err := k.distrKeeper.FundCommunityPool(ctx, sdk.NewCoins(daoRewardCoin), blockInflationAddr)
	if err != nil {
		return err
	}
	k.Logger(ctx).Debug("funded community pool", "amount", daoRewardCoin.String(), "from", blockInflationAddr)

	devRewardAmount := blockInflationDec.Mul(proportions.DeveloperRewards)
	devRewardCoin := sdk.NewCoin(k.stakingKeeper.BondDenom(ctx), devRewardAmount.TruncateInt())

	for _, w := range params.WeightedDeveloperRewardsReceivers {
		devRewardPortionCoins := sdk.NewCoins(k.GetProportions(ctx, devRewardCoin, w.Weight))
		if w.Address == "" {
			err := k.distrKeeper.FundCommunityPool(ctx, devRewardPortionCoins, blockInflationAddr)
			if err != nil {
				return err
			}
		} else {
			devRewardsAddr, err := sdk.AccAddressFromBech32(w.Address)
			if err != nil {
				return err
			}
			err = k.bankKeeper.SendCoins(ctx, blockInflationAddr, devRewardsAddr, devRewardPortionCoins)
			if err != nil {
				return err
			}
			k.Logger(ctx).Debug("sent coins to developer", "amount", devRewardPortionCoins.String(), "from", blockInflationAddr)
		}
	}

	return nil
}

// GetProportions gets the balance of the `MintedDenom` from minted coins
// and returns coins according to the `AllocationRatio`
func (k Keeper) GetProportions(ctx sdk.Context, mintedCoin sdk.Coin, ratio sdk.Dec) sdk.Coin {
	return sdk.NewCoin(mintedCoin.Denom, mintedCoin.Amount.ToDec().Mul(ratio).TruncateInt())
}