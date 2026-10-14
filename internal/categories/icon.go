package categories

import (
	"errors"
	"slices"
)

//
// Icon
//

type Icon string

const (
	IconPlayerPlay           Icon = "player-play"
	IconDeviceGamepad2       Icon = "device-gamepad-2"
	IconTrash                Icon = "trash"
	IconShoppingCart         Icon = "shopping-cart"
	IconShoppingBag          Icon = "shopping-bag"
	IconCar                  Icon = "car"
	IconBus                  Icon = "bus"
	IconMoped                Icon = "moped"
	IconMotorbike            Icon = "motorbike"
	IconFerry                Icon = "ferry"
	IconBike                 Icon = "bike"
	IconScooter              Icon = "scooter"
	IconHelicopter           Icon = "helicopter"
	IconTruck                Icon = "truck"
	IconCamper               Icon = "camper"
	IconFileInvoice          Icon = "file-invoice"
	IconInvoice              Icon = "invoice"
	IconCashBanknote         Icon = "cash-banknote"
	IconCashCoin             Icon = "cash-coin"
	IconReceipt              Icon = "receipt"
	IconReceipt2             Icon = "receipt-2"
	IconReceiptBitcoin       Icon = "receipt-bitcoin"
	IconReceiptDollar        Icon = "receipt-dollar"
	IconReceiptEuro          Icon = "receipt-euro"
	IconReceiptPound         Icon = "receipt-pound"
	IconReceiptRupee         Icon = "receipt-rupee"
	IconReceiptYen           Icon = "receipt-yen"
	IconReceiptYuan          Icon = "receipt-yuan"
	IconSportBilliard        Icon = "sport-billiard"
	IconExerciseBall         Icon = "exercise-ball"
	IconBallBaseball         Icon = "ball-baseball"
	IconWallet               Icon = "wallet"
	IconCreditCard           Icon = "credit-card"
	IconBallAmericanFootball Icon = "ball-american-football"
	IconBallVolleyball       Icon = "ball-volleyball"
	IconPokeball             Icon = "pokeball"
	IconWreckingBall         Icon = "wrecking-ball"
	IconHammerDrill          Icon = "hammer-drill"
	IconBalloon              Icon = "balloon"
	IconBallpen              Icon = "ballpen"
	IconBallFootball         Icon = "ball-football"
	IconBallBasketball       Icon = "ball-basketball"
	IconBarbell              Icon = "barbell"
	IconDeviceMobile         Icon = "device-mobile"
	IconCrystalBall          Icon = "crystal-ball"
	IconBell                 Icon = "bell"
	IconPill                 Icon = "pill"
	IconGrill                Icon = "grill"
	IconDirections           Icon = "directions"
	IconBeach                Icon = "beach"
	IconGlobe                Icon = "globe"
	IconCompass              Icon = "compass"
	IconPlane                Icon = "plane"
	IconLuggage              Icon = "luggage"
	IconWorldMap             Icon = "world-map"
	IconGavel                Icon = "gavel"
	IconLanguageKatakana     Icon = "language-katakana"
	IconLanguageHiragana     Icon = "language-hiragana"
	IconMoneybag             Icon = "moneybag"
	IconPigMoney             Icon = "pig-money"
	IconCoins                Icon = "coins"
	IconScale                Icon = "scale"
	IconPlanet               Icon = "planet"
	IconEyeglass2            Icon = "eyeglass-2"
	IconFlame                Icon = "flame"
	IconHome2                Icon = "home-2"
	IconHome                 Icon = "home"
	IconHorse                Icon = "horse"
	IconPlant                Icon = "plant"
	IconIroning              Icon = "ironing"
	IconCactus               Icon = "cactus"
	IconShirt                Icon = "shirt"
)

var iconCatalog = []Icon{
	IconPlayerPlay,
	IconDeviceGamepad2,
	IconTrash,
	IconShoppingCart,
	IconShoppingBag,
	IconCar,
	IconBus,
	IconMoped,
	IconMotorbike,
	IconFerry,
	IconBike,
	IconScooter,
	IconHelicopter,
	IconTruck,
	IconCamper,
	IconFileInvoice,
	IconInvoice,
	IconCashBanknote,
	IconCashCoin,
	IconReceipt,
	IconReceipt2,
	IconReceiptBitcoin,
	IconReceiptDollar,
	IconReceiptEuro,
	IconReceiptPound,
	IconReceiptRupee,
	IconReceiptYen,
	IconReceiptYuan,
	IconSportBilliard,
	IconExerciseBall,
	IconBallBaseball,
	IconWallet,
	IconCreditCard,
	IconBallAmericanFootball,
	IconBallVolleyball,
	IconPokeball,
	IconWreckingBall,
	IconHammerDrill,
	IconBalloon,
	IconBallpen,
	IconBallFootball,
	IconBallBasketball,
	IconBarbell,
	IconDeviceMobile,
	IconCrystalBall,
	IconBell,
	IconPill,
	IconGrill,
	IconDirections,
	IconBeach,
	IconGlobe,
	IconCompass,
	IconPlane,
	IconLuggage,
	IconWorldMap,
	IconGavel,
	IconLanguageKatakana,
	IconLanguageHiragana,
	IconMoneybag,
	IconPigMoney,
	IconCoins,
	IconScale,
	IconPlanet,
	IconEyeglass2,
	IconFlame,
	IconHome2,
	IconHome,
	IconHorse,
	IconPlant,
	IconIroning,
	IconCactus,
	IconShirt,
}

var ErrIconInvalid = errors.New("invalid icon")

func NewIcon(value string) (Icon, error) {
	var (
		err  error
		icon Icon
	)

	icon = Icon(value)
	err = icon.Validate()
	return icon, err
}

// Icons lists every known icon in the order they are offered.

func Icons() []Icon {
	return slices.Clone(iconCatalog)
}

func (i Icon) Validate() error {
	var err error

	if !slices.Contains(iconCatalog, i) {
		err = ErrIconInvalid
	}

	return err
}
