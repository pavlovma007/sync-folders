package dht

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/traversal"
	"github.com/anacrolix/torrent/bencode"
)

// DHTClient — интерфейс для DHT-операций (put/get манифеста).
type DHTClient interface {
	Put(pub, priv []byte, salt string, seq int64, value []byte) error
	Get(pub []byte, salt string) ([]byte, int64, error)
	Close() error
}

// Проверка что *Client реализует DHTClient
var _ DHTClient = (*Client)(nil)

// Client — обёртка над anacrolix/dht для BEP-44 mutable items.
type Client struct {
	server *dht.Server
	// Локальное хранилище для put/get. Хранит *bep44.Wrapper (а не сырой
	// Memory): сервер внутри оборачивает Store в тот же Wrapper, который
	// отбрасывает items с нулевым created как "просроченные", поэтому писать
	// мимо Wrapper нельзя — иначе положенный item нельзя отдать по сети.
	store   *bep44.Wrapper
	timeout time.Duration
	// done закрывается в Close, чтобы остановить горутину логирования
	// прогресса bootstrap (не допускает утечку при раннем выходе).
	done chan struct{}
}

// NewClient создаёт DHT-клиент, подключённый к Mainline DHT.
func NewClient() (*Client, error) {
	exp := time.Hour * 24 // храним сутки
	mem := bep44.NewMemory()

	// Клиент пишет/читает через bep44.Wrapper, а не напрямую в Memory:
	// сервер внутри себя оборачивает Store в тот же Wrapper (см. NewServer),
	// который отбрасывает items с нулевым временем created как "просроченные".
	// Без общего Wrapper положенный локально item нельзя было бы отдать
	// по сети другим узлам.
	cfg := dht.ServerConfig{
		Store:       mem,
		Exp:         exp,
		NoSecurity:  true, // как NewDefaultServerConfig; иначе ноды с не-secure ID не попадают в таблицу
		StartingNodes: func() ([]dht.Addr, error) {
			return dht.GlobalBootstrapAddrs("udp")
		}, // Mainline DHT bootstrap
	}

	s, err := dht.NewServer(&cfg)
	if err != nil {
		return nil, fmt.Errorf("dht new server: %w", err)
	}

	c := &Client{
		server:  s,
		store:   bep44.NewWrapper(mem, exp),
		timeout: 30 * time.Second,
		done:    make(chan struct{}),
	}
	c.logBootstrap()
	return c, nil
}

// logBootstrap запускает фоновую горутину, которая раз в 3 секунды логирует
// прогресс bootstrap таблицы маршрутизации: сколько нод найдено и сколько из
// них "good" (ответили на последний запрос). Логирование прекращается, когда
// набрано достаточно good-нод (20) или когда клиент закрыт.
func (c *Client) logBootstrap() {
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-c.done:
				return
			case <-ticker.C:
				st := c.server.Stats()
				if st.Nodes > 0 {
					log.Printf("[dht] bootstrap: %d nodes (%d good)", st.Nodes, st.GoodNodes)
				}
				if st.GoodNodes >= 20 {
					log.Printf("[dht] bootstrap: done (%d good nodes)", st.GoodNodes)
					return
				}
			}
		}
	}()
}

// Put публикует mutable item в DHT (без внешнего контекста).
// Сначала сохраняет item локально (быстрый путь), затем итеративным обходом
// (traversal) находит k ближайших к Target узлов и публикует item у каждого:
// Get-ом получает write token (BEP-44: token обязателен для Put), после чего
// отправляет Put. Локальное хранение — через Wrapper, чтобы item можно было
// отдать по сети (иначе сервер сочтёт его просроченным).
func (c *Client) Put(pub, priv []byte, salt string, seq int64, value []byte) error {
	item, err := bep44.NewItem(value, []byte(salt), seq, 0, ed25519.PrivateKey(priv))
	if err != nil {
		return fmt.Errorf("dht put create item: %w", err)
	}

	// Step 1: сохраняем локально (в т.ч. для отдачи по сети).
	if err := c.store.Put(item); err != nil {
		return fmt.Errorf("dht put store: %w", err)
	}

	// Step 2: публикуем в сеть итеративным lookup'ом.
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	target := item.Target() // bep44.Target и krpc.ID — оба [20]byte

	op := traversal.Start(traversal.OperationInput{
		Target: krpc.ID(target),
		K:      8,
		DoQuery: func(ctx context.Context, addr krpc.NodeAddr) traversal.QueryResult {
			// 2a. Get → write token.
			getQR := c.server.Get(ctx, dht.NewAddr(addr.UDP()), target, nil, dht.QueryRateLimiting{})
			if r := getQR.Reply.R; r == nil || r.Token == nil {
				return getQR.TraversalQueryResult(addr)
			}
			token := *getQR.Reply.R.Token

			// 2b. Put item с полученным токеном.
			putQR := c.server.Put(ctx, dht.NewAddr(addr.UDP()), item.ToPut(), token, dht.QueryRateLimiting{})
			// Успешность каждого put'а не отслеживаем: локальная копия уже
			// сохранена, а сетевая публикация — best effort (см. BEP-44).
			_ = putQR.ToError()
			return getQR.TraversalQueryResult(addr)
		},
	})

	nodes, err := c.server.TraversalStartingNodes()
	if err != nil {
		// Пустая таблица и нет StartingNodes — traversal сразу остановится.
		nodes = nil
	}
	op.AddNodes(nodes)

	select {
	case <-op.Stalled():
		op.Stop()
	case <-ctx.Done():
		op.Stop()
	}
	<-op.Stopped()

	return nil
}

// Get получает mutable item из DHT (без внешнего контекста).
// Сначала ищет в локальном хранилище (быстрый путь), а если там нет —
// выполняет итеративный обход (traversal) по Mainline DHT: ищет k ближайших
// к Target узлов и забирает у них item с максимальным seq.
func (c *Client) Get(pub []byte, salt string) ([]byte, int64, error) {
	var pubKey [32]byte
	copy(pubKey[:], pub)
	target := bep44.MakeMutableTarget(pubKey, []byte(salt))

	// Step 1: локальное хранилище (быстрый путь).
	if item, err := c.store.Get(target); err == nil {
		if v, ok := itemValue(item.V); ok {
			return v, item.Seq, nil
		}
	}

	// Step 2: итеративный lookup в Mainline DHT.
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	var (
		mu        sync.Mutex
		bestSeq   int64 = -1
		bestValue []byte
	)

	op := traversal.Start(traversal.OperationInput{
		Target: krpc.ID(target), // bep44.Target и krpc.ID — оба [20]byte
		K:      8,
		DoQuery: func(ctx context.Context, addr krpc.NodeAddr) traversal.QueryResult {
			qr := c.server.Get(ctx, dht.NewAddr(addr.UDP()), target, nil, dht.QueryRateLimiting{})
			if r := qr.Reply.R; r != nil && r.V != nil && r.Seq != nil {
				// BEP-44 mutable get: R.V — bencoded значение, R.K — ed25519
				// публичный ключ, R.Sig — подпись, R.Seq — номер версии.
				// Проверяем ключ и подпись, чтобы не доверять случайному узлу.
				if r.K == pubKey && bep44.Verify(r.K[:], []byte(salt), *r.Seq, []byte(r.V), r.Sig[:]) {
					var v []byte
					if err := bencode.Unmarshal([]byte(r.V), &v); err == nil {
						mu.Lock()
						if *r.Seq > bestSeq {
							bestSeq = *r.Seq
							bestValue = v
						}
						mu.Unlock()
					}
				}
			}
			return qr.TraversalQueryResult(addr)
		},
	})

	nodes, err := c.server.TraversalStartingNodes()
	if err != nil {
		// Пустая таблица и нет StartingNodes — traversal сразу остановится,
		// вернём "not found".
		nodes = nil
	}
	op.AddNodes(nodes)

	select {
	case <-op.Stalled():
		op.Stop()
	case <-ctx.Done():
		op.Stop()
	}
	<-op.Stopped()

	mu.Lock()
	defer mu.Unlock()
	if bestSeq >= 0 {
		return bestValue, bestSeq, nil
	}
	return nil, 0, fmt.Errorf("dht get: item not found")
}

// itemValue нормализует bencode-значение V из BEP-44 item в []byte.
// Локально созданный item имеет V = []byte (см. bep44.NewItem), а принятый
// по сети (через put query) — V = string: bencode-декодер распаковывает
// байтовую строку в string при распаковке в interface{}.
func itemValue(v interface{}) ([]byte, bool) {
	switch x := v.(type) {
	case []byte:
		return x, true
	case string:
		return []byte(x), true
	}
	return nil, false
}

// Close останавливает DHT-сервер и фоновые горутины (в т.ч. логирование
// bootstrap). Безопасен для клиентов, созданных в тестах без сервера.
func (c *Client) Close() error {
	if c.done != nil {
		close(c.done)
	}
	if c.server != nil {
		c.server.Close()
	}
	return nil
}

// Nodes возвращает количество нод в таблице маршрутизации.
// Полезно для диагностики bootstrap-состояния.
func (c *Client) Nodes() int {
	return c.server.Stats().Nodes
}

// NewTestPair создаёт два соединённых DHT-сервера для тестов.
func NewTestPair() (*TestDHT, *TestDHT, error) {
	store1 := bep44.NewMemory()
	store2 := bep44.NewMemory()

	s1, err := dht.NewServer(&dht.ServerConfig{
		Store:       store1,
		Exp:         time.Hour,
		WaitToReply: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("server 1: %w", err)
	}

	s2, err := dht.NewServer(&dht.ServerConfig{
		Store:       store2,
		Exp:         time.Hour,
		WaitToReply: true,
	})
	if err != nil {
		s1.Close()
		return nil, nil, fmt.Errorf("server 2: %w", err)
	}

	t1 := &TestDHT{server: s1, store: store1}
	t2 := &TestDHT{server: s2, store: store2}

	return t1, t2, nil
}

// TestDHT — тестовый DHT-клиент с прямым доступом к серверу.
type TestDHT struct {
	server *dht.Server
	store  bep44.Store
}

func (td *TestDHT) Put(pub, priv []byte, salt string, seq int64, value []byte) error {
	item, err := bep44.NewItem(value, []byte(salt), seq, 0, ed25519.PrivateKey(priv))
	if err != nil {
		return fmt.Errorf("test put create: %w", err)
	}
	return td.store.Put(item)
}

func (td *TestDHT) Get(pub []byte, salt string) ([]byte, int64, error) {
	var pubKey [32]byte
	copy(pubKey[:], pub)
	target := bep44.MakeMutableTarget(pubKey, []byte(salt))
	item, err := td.store.Get(target)
	if err != nil {
		return nil, 0, fmt.Errorf("test get: %w", err)
	}
	return item.V.([]byte), item.Seq, nil
}

func (td *TestDHT) Addr() string {
	return td.server.Addr().String()
}

func (td *TestDHT) Close() error {
	td.server.Close()
	return nil
}
