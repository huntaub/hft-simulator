package main

import (
	"fmt"
	"math/rand"
	"sort"
	"time"
)

type traderBook struct {
	SellBook sellOrderBook
	BuyBook  buyOrderBook
}

func (t *traderBook) filledOrder(o *order, q int) {
	if o.Sell {
		for i, v := range t.SellBook {
			if v.ID == o.ID {
				v.Quantity -= q
				if v.Quantity == 0 {
					t.SellBook = append(t.SellBook[:i], t.SellBook[i+1:]...)
				}
				return
			}
		}
	} else {
		for i, v := range t.BuyBook {
			if v.ID == o.ID {
				v.Quantity -= q
				if v.Quantity == 0 {
					t.BuyBook = append(t.BuyBook[:i], t.BuyBook[i+1:]...)
				}
				return
			}
		}
	}
}

func (t *traderBook) newOrder(o *order) {
	copiedOrder := &order{}
	*copiedOrder = *o

	if o.Sell {
		t.SellBook = append(t.SellBook, copiedOrder)
		sort.Stable(t.SellBook)
	} else {
		t.BuyBook = append(t.BuyBook, copiedOrder)
		sort.Stable(t.BuyBook)
	}
}

func (t *traderBook) Bid() float64 {
	if len(t.BuyBook) == 0 {
		return -1
	}

	return t.BuyBook[0].Price
}

func (t *traderBook) Ask() float64 {
	if len(t.SellBook) == 0 {
		return -1
	}

	return t.SellBook[0].Price
}

type simpleTrader struct {
	Cash  float64
	Stock int
	Mean  float64

	books       *traderBook
	Outstanding map[float64]*order
}

func newSimpleTrader(mean float64) *simpleTrader {
	price := (rand.Float64() * 80) + 60

	t := &simpleTrader{
		Mean:        mean,
		Cash:        2000 - (price * 10),
		Stock:       10,
		books:       &traderBook{},
		Outstanding: make(map[float64]*order),
	}

	connectionChannel <- &connectionInfo{
		trader: t,
		Open:   true,
	}

	return t
}

func (s *simpleTrader) writeMessage(typ string, data interface{}) {
	if typ == "newOrder" {
		s.newOrder(data.(*order))
	} else if typ == "cancelledOrder" {

	} else if typ == "filledOrder" {
		m := data.(map[string]interface{})
		s.filledOrder(
			m["sellOrder"].(*order),
			m["buyOrder"].(*order),
			m["quantity"].(int),
			m["price"].(float64),
		)
	} else {
		fmt.Println("Didn't respond to type...", typ)
	}
}

func (s *simpleTrader) placeOrder(o *order) {
	s.Outstanding[o.ID] = o
	go func() {
		listChannel <- o
	}()
}

func (s *simpleTrader) makeOrder(q int, p float64, sell bool) *order {
	return &order{
		ID:       rand.Float64(),
		Quantity: q,
		Price:    p,
		Sell:     sell,
		Date:     time.Now(),
		Owner:    s,
	}
}

func (s *simpleTrader) filledOrder(sell *order, buy *order, q int, p float64) {
	s.books.filledOrder(sell, q)
	s.books.filledOrder(buy, q)

	// fmt.Println("Filled order sell", sell.ID, "buy", buy.ID)
	if _, sok := s.Outstanding[sell.ID]; sok {
		s.Cash += float64(q) * p
		delete(s.Outstanding, sell.ID)

		bid := s.books.Bid() + 1
		if bid == 0 {
			bid = 99.99
		}

		if bid < s.Mean {
			fmt.Println("REBUYING STOCK AT", bid)
			s.placeOrder(s.makeOrder(q, bid, false))
			s.Cash -= float64(q) * bid
		} else {
			fmt.Println("Won't rebuy stock...", bid)
		}
	} else if _, bok := s.Outstanding[buy.ID]; bok {
		s.Stock += q
		delete(s.Outstanding, buy.ID)

		ask := s.books.Ask() - 1
		if ask == -2 {
			ask = 100.01
		}

		if ask > s.Mean {
			fmt.Println("RESELLING STOCK AT", ask)
			s.placeOrder(s.makeOrder(q, ask, true))
			s.Stock -= q
		} else {
			fmt.Println("Won't resell stock...", ask)
		}

	}

	fmt.Println("Trader Update", s.Cash, "dollars", s.Stock, "shares")
}

func (s *simpleTrader) newOrder(o *order) {
	s.books.newOrder(o)

	if (o.Sell && o.Price < s.Mean) || (!o.Sell && o.Price > s.Mean) {
		newOrder := &order{
			ID:       rand.Float64(),
			Quantity: o.Quantity,
			Price:    o.Price,
			Date:     time.Now(),
			Sell:     !o.Sell,
			Owner:    s,
		}
		s.placeOrder(newOrder)
		if newOrder.Sell {
			s.Stock -= o.Quantity
		} else {
			s.Cash -= float64(o.Quantity) * o.Price
		}

		s.Outstanding[newOrder.ID] = newOrder
	}
}

type marketMakerTrader struct {
	Cash  float64
	Stock int
	Mean  float64

	ToWait time.Duration
	Timer  time.Timer

	Outstanding map[float64]*order
}

func newMarketMakerTrader(mean float64) *marketMakerTrader {
	t := &marketMakerTrader{
		Mean:        mean,
		Outstanding: make(map[float64]*order),
	}

	connectionChannel <- &connectionInfo{
		trader: t,
		Open:   true,
	}

	return t
}

func (s *marketMakerTrader) writeMessage(typ string, data interface{}) {
	if typ == "newOrder" {
		s.newOrder(data.(*order))
	} else if typ == "cancelledOrder" {

	} else if typ == "filledOrder" {
		m := data.(map[string]interface{})
		s.filledOrder(
			m["sellOrder"].(*order),
			m["buyOrder"].(*order),
			m["quantity"].(int),
			m["price"].(float64),
		)
	}
}

func (s *marketMakerTrader) placeOrder(o *order) {
	s.Outstanding[o.ID] = o
	go func() {
		listChannel <- o
	}()
}

func (s *marketMakerTrader) filledOrder(sell *order, buy *order, q int, p float64) {
	if _, sok := s.Outstanding[sell.ID]; sok {
		s.Cash += float64(q) * p
		delete(s.Outstanding, sell.ID)
	} else if _, bok := s.Outstanding[buy.ID]; bok {
		s.Stock += q
		delete(s.Outstanding, buy.ID)
	}

	fmt.Println("Trader Update", s.Cash, "dollars", s.Stock, "shares")
}

func (s *marketMakerTrader) newOrder(o *order) {
	if (o.Sell && o.Price < s.Mean) || (!o.Sell && o.Price > s.Mean) {
		newOrder := &order{
			ID:       rand.Float64(),
			Quantity: o.Quantity,
			Price:    o.Price,
			Date:     time.Now(),
			Sell:     !o.Sell,
			Owner:    s,
		}
		s.placeOrder(newOrder)
		if newOrder.Sell {
			s.Stock -= o.Quantity
		} else {
			s.Cash -= float64(o.Quantity) * o.Price
		}

		s.Outstanding[newOrder.ID] = newOrder
	}
}

// listChannel *order
// connectionChannel *connectionInfo

// 2 strategies
// 1.
// Knows the distribution of values, 60 - 140, mean of 100
// Whenever he sees stock for sale, selling below 100 (more likely to be taken by other people)
// take this order first
// Resell the stock at 100
// Whenever buying order is above 100, he will sell to that order
// puts a buy order in at 100
// catch bid/ask <- narrow the gap if it exists for too long try to get it to 100

// 2.
// If no one has posted any orders in 3 seconds, he will be a "market maker"
// He will sell a stock at $120 and immediately buy it back at $120

// discount by every second 0.5% or 0.05%.
// dividend on the stock 0.2% added to cash
