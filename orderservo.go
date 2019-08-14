package main

import (
	"math/rand"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"github.com/binance-chain/go-sdk/client/basic"
	"github.com/binance-chain/go-sdk/client/query"
	"github.com/binance-chain/go-sdk/client/transaction"
	ctypes "github.com/binance-chain/go-sdk/common/types"
	"github.com/binance-chain/go-sdk/keys"
	"github.com/binance-chain/go-sdk/types/tx"
	"github.com/spf13/cobra"
)

var (
	km keys.KeyManager
	c basic.BasicClient
	q query.QueryClient
	lotsize int64 = 10000000000
	wantprice int64 = 9000
	tradingpair string = "ZCB-F00_BNB"
	seq int64
	wg1 sync.WaitGroup
	wg2 sync.WaitGroup
	sellquantity int64 = 10000000000
)

var rootCmd = &cobra.Command{
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("start")
	},
}

var buyCmd = &cobra.Command{
	Use: "buy",
	Short: "找到价格合适的卖单, 发送使它部分成交的买单",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("\x1b[93m start\x1b[0m \x1b[92m buy \x1b[0m")
		stop1 := false
		ch1 := make(chan interface{})
		asksgetter := newAskPriceChangeGetter()
		buyrunner := newPlaceBuyOrdersRunner(tradingpair, wantprice)
		go listener(&wg1, tradingpair, ch1, asksgetter, &stop1)
		go servo(&wg2, ch1, &stop1, buyrunner, &seq)
		select{}
	},
}

var sellCmd = &cobra.Command{
	Use: "sell",
	Short: "发送逐步提高价格的卖单",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("\x1b[93m start\x1b[0m \x1b[91m sell\x1b[0m")
		stop2 := false
		ch2 := make(chan interface{})
		mysellordergetter := newMySellOrderChangeGetter()
		sellrunner := newPlaceSellOrderRunner(tradingpair, sellquantity)
		go listener(&wg1, tradingpair, ch2, mysellordergetter, &stop2)
		go servo(&wg2, ch2, &stop2, sellrunner, &seq)
		select{}
	},
}

var buysellCmd = &cobra.Command{
	Use: "buysell",
	Short: "1. 找到价格合适的卖单, 发送使它部分成交的买单, 2. 发送逐步提高价格的卖单",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("\x1b[93m start\x1b[0m \x1b[92m buy\x1b[0m \x1b[91m sell\x1b[0m")
		stop1 := false
		ch1 := make(chan interface{})
		asksgetter := newAskPriceChangeGetter()
		buyrunner := newPlaceBuyOrdersRunner(tradingpair, wantprice)
		go listener(&wg1, tradingpair, ch1, asksgetter, &stop1)
		go servo(&wg2, ch1, &stop1, buyrunner, &seq)
		stop2 := false
		ch2 := make(chan interface{})
		mysellordergetter := newMySellOrderChangeGetter()
		sellrunner := newPlaceSellOrderRunner(tradingpair, sellquantity)
		go listener(&wg1, tradingpair, ch2, mysellordergetter, &stop2)
		go servo(&wg2, ch2, &stop2, sellrunner, &seq)
		select{}
	},
}

func init() {
	ctypes.Network = ctypes.TestNetwork
	c = basic.NewClient("testnet-dex.binance.org:443")
	q = query.NewClient(c)

	buyCmd.PersistentFlags().StringVarP(&tradingpair, "tradingpair", "t", "ZCB-F00_BNB", "trading pair")
	buyCmd.PersistentFlags().Int64VarP(&wantprice, "wantprice", "w", 9000, "want price")
	buyCmd.PersistentFlags().Int64VarP(&lotsize, "lotsize", "l", 10000000000, "单位交易量")  // TODO default 改为自动获取

	sellCmd.PersistentFlags().StringVarP(&tradingpair, "tradingpair", "t", "ZCB-F00_BNB", "trading pair")
	sellCmd.PersistentFlags().Int64VarP(&sellquantity, "sellquantity", "s", 10000000000, "sell quantity")

	buysellCmd.PersistentFlags().StringVarP(&tradingpair, "tradingpair", "t", "ZCB-F00_BNB", "trading pair")
	buysellCmd.PersistentFlags().Int64VarP(&wantprice, "wantprice", "w", 9000, "want price")
	buysellCmd.PersistentFlags().Int64VarP(&lotsize, "lotsize", "l", 10000000000, "单位交易量")
	buysellCmd.PersistentFlags().Int64VarP(&sellquantity, "sellquantity", "s", 10000000000, "sell quantity")

	rootCmd.AddCommand(buyCmd)
	rootCmd.AddCommand(sellCmd)
	rootCmd.AddCommand(buysellCmd)
}

func main() {
	km, _ = keys.NewMnemonicKeyManager("govern cancel early excite other fox canvas satoshi social shiver version inch correct web soap always water wine grid fashion voyage finish canal subject")
	acc, err := GetAccount()
	if err != nil {
		log.Fatal(err)
	}
	seq = acc.Sequence
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type Getter func (params ...interface{}) (interface{}, error)

type Runner func (input interface{}, seq *int64) (done bool, res string, err error)

// 获取订单本价格变化
func newAskPriceChangeGetter() Getter {
	return Getter(func (params ...interface{}) (res1 interface{}, err1 error) {
		defer func () {
			if e := recover(); e != nil {
				err1 = fmt.Errorf("get ask price change error")
				return
			}
		} ()
		log.Println("getAskPriceChange")
		tradingpair := params[0].(string)
		res, err :=  GetOpenOrders(tradingpair)
		if err != nil {
			return nil, err
		}
		m := params[1].(map[string]string)
		if len(res.Asks[0]) < 1 {
			return nil, fmt.Errorf("no sell orders.")
		}
		if m[tradingpair+"lastaskprice"] != res.Asks[0][0] {
			m["lastprice"] = res.Asks[0][0]
			return res, nil
		} else {
			return nil, fmt.Errorf("no price change.")
		}
	})
}

type mySellOrder struct {
	status string
	mem map[string]string
}

// 获取我的订单状态变化
func newMySellOrderChangeGetter() Getter {
	return Getter(func (params ...interface{}) (res1 interface{}, err1 error) {
		defer func () {
			if e := recover(); e != nil {
				err1 = fmt.Errorf("get my sell order change error")
				return
			}
		} ()
		log.Println("getMySellOrderChange")
		tradingpair := params[0].(string)
		m := params[1].(map[string]string)
		id := m[tradingpair+"mysellorder"]
		if id == "" {
			return mySellOrder{status:"no order", mem:m}, nil
		} else {
			res, err := GetOrder(id)
			if err != nil {
				return mySellOrder{}, err
			}
			return mySellOrder{status:res.Status, mem:m}, nil
		}
	})
}

// 根据订单本数据下买单
func newPlaceBuyOrdersRunner(tradingpair string, wantprice int64) Runner {
	return Runner(func (input interface{}, seq *int64) (placed bool, orderid string, err error) {
		marketOrders := input.(*ctypes.MarketDepth)
		if len(marketOrders.Asks) < 1 {
			return false, "", fmt.Errorf("no sell orders.")
		}
		price8, _ := ctypes.Fixed8DecodeString(marketOrders.Asks[0][0])
		price := price8.ToInt64()
		orderamount8, _ := ctypes.Fixed8DecodeString(marketOrders.Asks[0][1])
		orderamount := orderamount8.ToInt64()
		log.Printf("\n\x1b[104m 卖单价格: %v    目标价格: %v \x1b[0m\n", price, wantprice)
		if float64(price) < float64(wantprice) {
			fmt.Printf("\n买买买\n")
			fmt.Printf("order amount: %v\n", orderamount)
			if orderamount > lotsize {
				//buy orderamount * 0.8
				amt := int64(float64(orderamount)*0.8)
				amt = amt - amt % lotsize
				res, err := PlaceOrder(tradingpair, 1, price, amt, seq)
				if err != nil {
					return false, "", err
				}
				*seq ++
				log.Printf("\x1b[95m 成交量: %v    成交价格: %v \x1b[0m\n", amt, price)
				return true, res.OrderId, nil
			} else {
				//buy lotsize
				res, err := PlaceOrder(tradingpair, 1, price, lotsize, seq)
				if err != nil {
					return false, "", err
				}
				*seq ++
				log.Printf("\x1b[34m 成交量: %v    成交价格: %v \x1b[0m\n", lotsize, price)
				return true, res.OrderId, nil
			}
		}
		//return false, "", fmt.Errorf("price does not satisfy.")
		return false, "", nil
	})
}

func newPlaceSellOrderRunner(tradingpair string, quantity int64) Runner {
	return Runner(func (input interface{}, seq *int64) (placed bool, orderid string, err error) {
		s := input.(mySellOrder)
		if s.status == "no order" || s.status == "FullyFill" {
			log.Printf("my sell order: %+v", s)
			fmt.Printf("\n卖卖卖\n")
			// sell
			pricestr := s.mem[tradingpair+"sellprice"]
			price, _ := strconv.ParseInt(pricestr, 10, 64)
			price = price + 1000
			res, err := PlaceOrder(tradingpair, 2, price, quantity, seq)
			if err != nil {
				return false, "", err
			}
			*seq ++
			s.mem[tradingpair+"mysellorder"] = res.OrderId
			return true, res.OrderId, nil
		}
		return false, "", nil
	})
}

func listener(wg *sync.WaitGroup, tradingpair string, ch chan<- interface{}, getter Getter, stop *bool) {
	var m map[string]string = make(map[string]string)
	for !*stop {
		res, err := getter(tradingpair, m)
		log.Printf("\x1b[93m res: %v      err: %v \x1b[0m\n", res, err)
		if err != nil || res == nil {
			time.Sleep(time.Duration(5) * time.Second)
			continue
		}
		ch <- res
		time.Sleep(time.Duration(1000) * time.Millisecond)
	}
	wg.Done()
	defer close(ch)
}

func servo(wg *sync.WaitGroup, ch <-chan interface{}, stop *bool, runner Runner, seq *int64) {
	for !*stop {
		input, hasMore := <-ch
		if hasMore {
			wg.Add(1)
			placed, orderid, err := runner(input, seq)
			wg.Done()
			if placed {
				log.Printf("\n\x1b[32m order is placed:%v \x1b[0m\n", orderid)
				continue
			}
			if err != nil {
				log.Printf("\n\x1b[33m cannot place order:%v \x1b[0m\n", err)
				continue
			}
		}
		time.Sleep(time.Duration(200+rand.Intn(1000)) * time.Millisecond)
	}
}

func PlaceOrder(tradingpair string, side int8, price, quantity int64, seq *int64) (*transaction.CreateOrderResult, error) {
	// TODO 过期时间
	// 手动设置sequence
	t := transaction.NewClient("Binance-Chain-Nile", km, q, c)
	tmp := strings.Split(tradingpair, "_")
	opt := transaction.Option(func(txmsg *tx.StdSignMsg) *tx.StdSignMsg {
		txmsg.Sequence = *seq
		return txmsg
	})
	res, err := t.CreateOrder(tmp[0], tmp[1], side, price, quantity, true, opt)
	if err != nil {
		return res, err
	}
	return res, nil
}

func GetOrder(id string) (*ctypes.Order, error) {
	return q.GetOrder(id)
}

func GetKlines(tradingpair string) ([]ctypes.Kline, error) {
	lim := uint32(1)
	start := (time.Now().Unix() - 7200) * 1000
	end := (time.Now().Unix()) * 1000
	query := &ctypes.KlineQuery{tradingpair, "1m", &lim, &start, &end}
	res, err := q.GetKlines(query)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func GetOpenOrders(tradingpair string) (*ctypes.MarketDepth, error) {
	lim := uint32(5)
	query := &ctypes.DepthQuery{Symbol:tradingpair, Limit:&lim}
	return q.GetDepth(query)
}

func GetAccount() (*ctypes.BalanceAccount, error) {
	return q.GetAccount(km.GetAddr().String())
}
