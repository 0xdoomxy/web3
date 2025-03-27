> 以上为作者从零开始对Ethereum源码进行剖析，在目前绝大多数的资料中，只是对ethereum的架构做了一个科普性的介绍，没有对ethereum的实现做过多阐述。作为开发者而言，了解web3的前提是必须熟悉一条区块链的底层实现，因为他代表了一种P2P的新范式,这与以往web2的开发模式大相径庭，这更多的是一种理念的创新。不管是Ethereum还是Bitcoin或者说是其他的Layer2以及Solana,其最终都会遵守一个最基本的开发理念:去中心化。作者想通过对Ethereum源码的分析来对当前的所有链的底层原理有一个通俗的理解,同时想借用这系列的文章与更多web3的工作者交流技术。[源码版本下载](https://github.com/0xdoomxy/go-ethereum)


[go-ethereum源码剖析:geth的启动流程](https://www.0xdoomxy.top/#/article/4) 讲到geth console最终会执行localConsole这个函数，并且附带用户参数的上下文cCtx

```go

// localConsole starts a new geth node, attaching a JavaScript console to it at the
// same time.
func localConsole(ctx *cli.Context) error {
	// 如果是主网的话会填充默认的缓存信息，以及geth 程序的系统metrcis的注册以及收集
	prepare(ctx)
    //初始化ethereum节点,主要是对本地的区块链信息的构建(包括两类交易池blob pool和transaction pool,miner，gas price预言机,同时初始化该节点最新的区块数据和本地数据库)以及账户管理的服务注册(目前支持公私钥对、USB接口以及ID card接口)、geth对外暴露相关的服务接口(包括http、websocket、rpc),geth还会开启IPC通信(目前没有找到会用在什么地方),这是了解full node启动的重要函数之一。
	stack := makeFullNode(ctx)
        //启动由makeFullNode注册的相关服务。
	startNode(ctx, stack, true)
	defer stack.Close()

	// Attach to the newly started node and create the JavaScript console.
	client := stack.Attach()
	config := console.Config{
		DataDir: utils.MakeDataDir(ctx),
		DocRoot: ctx.String(utils.JSpathFlag.Name),
		Client:  client,
		Preload: utils.MakeConsolePreloads(ctx),
	}
	console, err := console.New(config)
	if err != nil {
		return fmt.Errorf("failed to start the JavaScript console: %v", err)
	}
	defer console.Stop(false)

	// If only a short execution was requested, evaluate and return.
	if script := ctx.String(utils.ExecFlag.Name); script != "" {
		console.Evaluate(script)
		return nil
	}

	// Track node shutdown and stop the console when it goes down.
	// This happens when SIGTERM is sent to the process.
	go func() {
		stack.Wait()
		console.StopInteractive()
	}()

	// Print the welcome screen and enter interactive mode.
	console.Welcome()
	console.Interactive()
	return nil
}


```

对于localConsole函数来讲，要了解ethereum启动的时候具体干了什么，最重要的是把握makeFullNode和startNode函数,通过这个函数能够初始化ethereum节点,主要是对本地的区块链信息的构建(包括两类交易池blob pool和transaction pool,miner，gas price预言机,同时初始化该节点最新的区块数据和本地数据库)以及账户管理的服务注册(目前支持公私钥对、USB接口以及ID card接口)、geth对外暴露相关的服务接口(包括http、websocket、rpc),geth还会开启IPC通信(目前没有找到会用在什么地方)。



makeFullNode以及startNode都是对上述结构体进行操作,对于ethereum来说,我们常见的操作一般是发起交易、参与共识出块，访问当前区块链的整体信息,从底层来讲，他还会涉及到P2P节点的通信相关问题。但总体而言，Full Node都会对外界提供一种方式让用户进行上述过程,我们需要找到他具体为外界提供了哪些操作，这方便我们从接口出发去剖析底层区块链的运行原理，这是必要的。

为了逐步了解Full Node对于用户提供的所有可访问的接口，作者找到了[ethereum的JSON-RPC文档](https://ethereum.github.io/execution-apis/api-documentation/)作为参考去分析。在startNode阶段,geth会开启上述的相关服务以便为用户提供上述服务,startNode会操控stack这个数据结构达到上述目的,为了后面的源码分析，我们需要了解Node这个结构体,初步判断Node会通过什么样的方式去暴露上述接口

```go
// Node is a container on which services can be registered.
type Node struct {
	eventmux      *event.TypeMux
        //这里的配置包括默认的配置+用户自定义的配置信息
	config        *Config
        // 管理用户账号的 manager
	accman        *accounts.Manager
	log           log.Logger
	keyDir        string        // key store directory
	keyDirTemp    bool          // If true, key directory will be removed by Stop
        //文件锁
	dirLock       *flock.Flock  // prevents concurrent use of instance directory
        //判断服务是否停止的信号量，这是很常见的用法,grpc的实现中也会利用到这一点
	stop          chan struct{} // Channel to wait for termination notifications
        // P2P服务
	server        *p2p.Server   // Currently running P2P networking layer
	startStopLock sync.Mutex    // Start/Stop are protected by an additional lock
        // node现在处于的阶段，总共有三个阶段,初始化,运行中，结束
	state         int           // Tracks state of node lifecycle
	lock          sync.Mutex
        // 下面的lifecycles以及rpcAPIS以及多个httpServer与full node对外界提供服务相关
	lifecycles    []Lifecycle // All registered backends, services, and auxiliary services that have a lifecycle
	rpcAPIs       []rpc.API   // List of APIs currently provided by the node
	http          *httpServer //
	ws            *httpServer //
	httpAuth      *httpServer //
	wsAuth        *httpServer //
	ipc           *ipcServer  // Stores information about the ipc http server
	inprocHandler *rpc.Server // In-process RPC request handler to process the API requests
        // ethereum database的包装器，用于加强对database的关闭的方法
	databases map[*closeTrackingDB]struct{} // All open databases
}

```


在通过makeFullNode将geth所有相关的服务注册就绪后,startNode会统一启动这些注册的服务以暴露在http、websocket、rpc下,我们需要重点关注在makeFullNode中究竟注册了哪些服务。在此之前，我们需要关注在startNode方法下会将makeFullNode的哪些东西进行start,这是非常关键的，这有助于我们去反向推导我们在makeFullNode中应该关注哪些字段或者函数。


<!--EndFragment-->

![start_node](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/start%20node%20function.png)

<!--EndFragment-->

![base_start_node](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/start%20node%20function%20base.png)
<!--EndFragment-->

![node_start](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/node%20start.png)




<!--EndFragment-->

![endpoint](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/endpoint.png)
<!--EndFragment-->

![start_rpc](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/start%20rpc.png)

<!--EndFragment-->

![enablews](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/enable%20ws.png)

<!--StartFragment-->

![geth_start过程](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/geth%20start过程.png)



从上面的图来看，在startNode中会将lifecycle数组中的ethereum.Backend调用start进行启动,以及对rpcAPIs的相关服务进行注册(**我们应该了解到的是在startNode阶段,geth通过反射机制将rpc.API的继承结构体的对应方法注册到路由中**)。在后续对makeFullNode的分析中，我们应该重要关注这两个字段的改变,下面我们先来具体看一下makeFullNode如何初始化ethereum full node。

```go
// makeFullNode 需要核心关注两个点 makeConfigNode和utils.RegisterEthService,
// 前者主要是注册P2P节点相关的服务并以stack的形式进行返回
// 后端会注册各种ethereum相关的接口以便通过http、rpc、websocket、以及进程内部调用和IPC进行访问，
// 这些接口包括共识的相关接口,区块链历史数据同步相关接口、区块链相关数据访问接口
// 以及区块链管理的相关接口(通过导入的方式构建区块链等等)
func makeFullNode(ctx *cli.Context) *node.Node {
        //初始化P2P节点,,并初始化区块链中blob pool、transaction pool、gas price oracle,同时构建accoumt manager 
        //这里会尝试获取datadir文件夹下面的文件锁,保证同一时间对datadir下面的文件夹访问是互斥的
	stack, cfg := makeConfigNode(ctx)
	if ctx.IsSet(utils.OverrideCancun.Name) {
		v := ctx.Uint64(utils.OverrideCancun.Name)
		cfg.Eth.OverrideCancun = &v
	}
	if ctx.IsSet(utils.OverrideVerkle.Name) {
		v := ctx.Uint64(utils.OverrideVerkle.Name)
		cfg.Eth.OverrideVerkle = &v
	}
        //初始化以太坊节点，这里的初始化是具有真正意义上的初始化。他会通过节点已有的数据构建区块链,同时注册区块链相关的服务，也就是说，他是区块链运行的核心启动方法
	backend, eth := utils.RegisterEthService(stack, &cfg.Eth)

	// Create gauge with geth system and build information
	if eth != nil { // The 'eth' backend may be nil in light mode
		var protos []string
		for _, p := range eth.Protocols() {
			protos = append(protos, fmt.Sprintf("%v/%d", p.Name, p.Version))
		}
		metrics.NewRegisteredGaugeInfo("geth/info", nil).Update(metrics.GaugeInfoValue{
			"arch":      runtime.GOARCH,
			"os":        runtime.GOOS,
			"version":   cfg.Node.Version,
			"protocols": strings.Join(protos, ","),
		})
	}

	// 主要是用于过滤客户端订阅的日志,ethereum的日志不仅仅有通过智能合约的emit去输出的这一种方式，
    
	filterSystem := utils.RegisterFilterAPI(stack, backend, &cfg.Eth)

	// 是否让geth 提供 graphql的服务
	if ctx.IsSet(utils.GraphQLEnabledFlag.Name) {
		utils.RegisterGraphQLService(stack, backend, filterSystem, &cfg.Node)
	}
	// 暴露当前geth的blockchain的相关指标,包括gas、blockchain、连接的对等节点数量等等
	if cfg.Ethstats.URL != "" {
		utils.RegisterEthStatsService(stack, backend, cfg.Ethstats.URL)
	}
	// Configure full-sync tester service if requested
	if ctx.IsSet(utils.SyncTargetFlag.Name) {
		hex := hexutil.MustDecode(ctx.String(utils.SyncTargetFlag.Name))
		if len(hex) != common.HashLength {
			utils.Fatalf("invalid sync target length: have %d, want %d", len(hex), common.HashLength)
		}
		utils.RegisterFullSyncTester(stack, eth, common.BytesToHash(hex))
	}
        //注册POS共识机制,可以是由第三方提供的共识服务
	if ctx.IsSet(utils.DeveloperFlag.Name) {
		// Start dev mode.
		simBeacon, err := catalyst.NewSimulatedBeacon(ctx.Uint64(utils.DeveloperPeriodFlag.Name), eth)
		if err != nil {
			utils.Fatalf("failed to register dev mode catalyst service: %v", err)
		}
		catalyst.RegisterSimulatedBeaconAPIs(stack, simBeacon)
		stack.RegisterLifecycle(simBeacon)
	} else if ctx.IsSet(utils.BeaconApiFlag.Name) {
		// Start blsync mode.
		srv := rpc.NewServer()
		srv.RegisterName("engine", catalyst.NewConsensusAPI(eth))
		blsyncer := blsync.NewClient(utils.MakeBeaconLightConfig(ctx))
		blsyncer.SetEngineRPC(rpc.DialInProc(srv))
		stack.RegisterLifecycle(blsyncer)
	} else {
		// Launch the engine API for interacting with external consensus client.
		err := catalyst.Register(stack, eth)
		if err != nil {
			utils.Fatalf("failed to register catalyst service: %v", err)
		}
	}
	return stack
}
```

这里主要注意两点makeConfigNode和utils.RegisterEthService都会对ethereum full node 的服务进行构建，makeConfigNode主要是在于对P2P节点的功能进行构建,utils.RegisterEthService主要是对于ethereum blockchain相关的服务进行构建,其中包括初始化本地的database以便知晓当前的本节点的blockchain的情况以进行后续的同步和共识,同时还会对外界访问ethereum blockchain的相关接口进行注册(这个会在剖析utils.RegisterEthService这个方法的时候具体说明),

接下来让我们进入到stack, cfg := makeConfigNode(ctx)

> 通过utils.RegisterEthStatsService注册的服务暴露的相关指标详细可以参考官方给出的[dashboard](https://ethstats.dev/)

```go
// makeConfigNode loads geth configuration and creates a blank node instance.
func makeConfigNode(ctx *cli.Context) (*node.Node, gethConfig) {
        //加载ethereum的基本配置，包括ethereum的配置，以及P2P节点的配置甚至上述说到的EthStat和
        //普通Metrics的配置(ethereum通过配置文件的方式进行统一配置,支持toml格式)
	cfg := loadBaseConfig(ctx)
        //初始化一个P2P节点，此节点仅仅是提供P2P服务,不会操作上层的ethereum服务,但是会注册node.apis()
        //到rpcAPIs
	stack, err := node.New(&cfg.Node)
	if err != nil {
		utils.Fatalf("Failed to create the protocol stack: %v", err)
	}
	// 创建account manager 的backend
	if err := setAccountManagerBackends(stack.Config(), stack.AccountManager(), stack.KeyStoreDir()); err != nil {
		utils.Fatalf("Failed to set account manager backends: %v", err)
	}
        //将ethereum相关的配置加入到cfg.Eth中
	utils.SetEthConfig(ctx, stack, &cfg.Eth)
	if ctx.IsSet(utils.EthStatsURLFlag.Name) {
		cfg.Ethstats.URL = ctx.String(utils.EthStatsURLFlag.Name)
	}
        //在cfg中生效用户传入的metrics相关的参数配置
	applyMetricConfig(ctx, &cfg)

	return stack, cfg
}


```

从makeConfigNode来看,通过node.New()创建了后续startNode的对象stack,前面我们说过，startNode主要是对于stack的lifestyle和rpcAPIS进行操作。node.New()方法中，通过node.rpcAPIs = append(node.rpcAPIs, node.apis()...)将node对应的apis写入rpcAPIS。下一步我们来看一下node.apis()包含哪个元素。

```go
/ apis returns the collection of built-in RPC APIs.
func (n *Node) apis() []rpc.API {
	return []rpc.API{
		{
			Namespace: "admin",
                        //与P2P节点加入有关
			Service:   &adminAPI{n},
		}, {
			Namespace: "debug",
                        //与profile的有关
			Service:   debug.Handler,
		}, {
			Namespace: "debug",
                        //提供版本信息和sha3接口
			Service:   &p2pDebugAPI{n},
		}, {
			Namespace: "web3",
                        //与节点发现有关
			Service:   &web3API{n},
		},
	}
}
```

想要了解上述四个元素未来会在startNode注册哪些接口,可以查看对应Service结构体所包含的方法即可,下面不做过多解释(这里rpc.API都只和P2P节点的服务相关)。


下面让我们接着来看一下utils.RegisterEthService方法。

> 当前对源码的剖析旨在于大体掌握geth的整体工作模式和设计，后续会针对各个重点模块具体剖析

```go

// RegisterEthService adds an Ethereum client to the stack.
// The second return value is the full node instance.
func RegisterEthService(stack *node.Node, cfg *ethconfig.Config) (*eth.EthAPIBackend, *eth.Ethereum) {
        //构建ethereum所需要的各种服务
	backend, err := eth.New(stack, cfg)
	if err != nil {
		Fatalf("Failed to register the Ethereum service: %v", err)
	}
        //这里注册对智能合约进行调试的相关接口
	stack.RegisterAPIs(tracers.APIs(backend.APIBackend))
	return backend.APIBackend, backend
}

```

```go
func New(stack *node.Node, config *ethconfig.Config) (*Ethereum, error) {
	// Ensure configuration values are compatible and sane
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	if config.Miner.GasPrice == nil || config.Miner.GasPrice.Sign() <= 0 {
		log.Warn("Sanitizing invalid miner gas price", "provided", config.Miner.GasPrice, "updated", ethconfig.Defaults.Miner.GasPrice)
		config.Miner.GasPrice = new(big.Int).Set(ethconfig.Defaults.Miner.GasPrice)
	}
	if config.NoPruning && config.TrieDirtyCache > 0 {
		if config.SnapshotCache > 0 {
			config.TrieCleanCache += config.TrieDirtyCache * 3 / 5
			config.SnapshotCache += config.TrieDirtyCache * 2 / 5
		} else {
			config.TrieCleanCache += config.TrieDirtyCache
		}
		config.TrieDirtyCache = 0
	}
	log.Info("Allocated trie memory caches", "clean", common.StorageSize(config.TrieCleanCache)*1024*1024, "dirty", common.StorageSize(config.TrieDirtyCache)*1024*1024)
//初始化ethereum数据库,这里需要注意的是,为了避免索引太大,以太坊用冷热数据这种类似的模式对数据进行持久化
//eth在底层建立了两种数据库,对于第一种基于LSM的KV的数据库,只保存前9w个区块的索引，剩下的会以另一种方式存储(通过idx文件以6字节为一个单位保存对内容的索引,
//对每个内容文件以最大为2GB进行切割，具体的内容包括区块信息(receipt、header和body),以及状态信息(account、history和storage))
	chainDb, err := stack.OpenDatabaseWithFreezer("chaindata", config.DatabaseCache, config.DatabaseHandles, config.DatabaseFreezer, "eth/db/chaindata/", false)
	if err != nil {
		return nil, err
	}
	scheme, err := rawdb.ParseStateScheme(config.StateScheme, chainDb)
	if err != nil {
		return nil, err
	}
	// Try to recover offline state pruning only in hash-based.
	if scheme == rawdb.HashScheme {
		if err := pruner.RecoverPruning(stack.ResolvePath(""), chainDb); err != nil {
			log.Error("Failed to recover state", "error", err)
		}
	}
	// 尝试在本地存储中获取区块链的信息(包括区块链的网络号、存款合约地址,以及各种重要提案开始影响的区块号(比如说The Dao事件,上海分叉,伊斯坦布尔分叉以及EIP150提案))
	chainConfig, err := core.LoadChainConfig(chainDb, config.Genesis)
	if err != nil {
		return nil, err
	}
        //这里的共识机制是设置POW而存在的，后面在2022年eth尝试将共识机制过渡为pos之后,此方法用作适配，并不会真正设置共识机制
	engine, err := ethconfig.CreateConsensusEngine(chainConfig, chainDb)
	if err != nil {
		return nil, err
	}
	networkID := config.NetworkId
	if networkID == 0 {
		networkID = chainConfig.ChainID.Uint64()
	}
	eth := &Ethereum{
		config:            config,
		chainDb:           chainDb,
		eventMux:          stack.EventMux(),
		accountManager:    stack.AccountManager(),
		engine:            engine,
		closeBloomHandler: make(chan struct{}),
		networkID:         networkID,
		gasPrice:          config.Miner.GasPrice,
		bloomRequests:     make(chan chan *bloombits.Retrieval),
		bloomIndexer:      core.NewBloomIndexer(chainDb, params.BloomBitsBlocks, params.BloomConfirms),
		p2pServer:         stack.Server(),
		discmix:           enode.NewFairMix(0),
		shutdownTracker:   shutdowncheck.NewShutdownTracker(chainDb),
	}
	bcVersion := rawdb.ReadDatabaseVersion(chainDb)
	var dbVer = "<nil>"
	if bcVersion != nil {
		dbVer = fmt.Sprintf("%d", *bcVersion)
	}
	log.Info("Initialising Ethereum protocol", "network", networkID, "dbversion", dbVer)

	if !config.SkipBcVersionCheck {
		if bcVersion != nil && *bcVersion > core.BlockChainVersion {
			return nil, fmt.Errorf("database version is v%d, Geth %s only supports v%d", *bcVersion, version.WithMeta, core.BlockChainVersion)
		} else if bcVersion == nil || *bcVersion < core.BlockChainVersion {
			if bcVersion != nil { // only print warning on upgrade, not on init
				log.Warn("Upgrade blockchain database version", "from", dbVer, "to", core.BlockChainVersion)
			}
			rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
		}
	}
	var (
		vmConfig = vm.Config{
			EnablePreimageRecording: config.EnablePreimageRecording,
		}
                //用于缓存Merkle Patricia Trie以及snapshot相关配置
		cacheConfig = &core.CacheConfig{
			TrieCleanLimit:      config.TrieCleanCache,
			TrieCleanNoPrefetch: config.NoPrefetch,
			TrieDirtyLimit:      config.TrieDirtyCache,
			TrieDirtyDisabled:   config.NoPruning,
			TrieTimeLimit:       config.TrieTimeout,
			SnapshotLimit:       config.SnapshotCache,
			Preimages:           config.Preimages,
			StateHistory:        config.StateHistory,
			StateScheme:         scheme,
		}
	)
	if config.VMTrace != "" {
		traceConfig := json.RawMessage("{}")
		if config.VMTraceJsonConfig != "" {
			traceConfig = json.RawMessage(config.VMTraceJsonConfig)
		}
		t, err := tracers.LiveDirectory.New(config.VMTrace, traceConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create tracer %s: %v", config.VMTrace, err)
		}
		vmConfig.Tracer = t
	}
	// Override the chain config with provided settings.
	var overrides core.ChainOverrides
	if config.OverrideCancun != nil {
		overrides.OverrideCancun = config.OverrideCancun
	}
	if config.OverrideVerkle != nil {
		overrides.OverrideVerkle = config.OverrideVerkle
	}
        //初始化以太坊核心的数据结构,这里面会在此之上对当前节点的区块链状态进行确认和验证。
        //当currentHeader的Number为0时,代表区块链还未同步完成。
        //在区块链中，对于一个区块来讲存在两种状态，分别是safe和Final,前者代表区块最终安全,这代表不会被分叉(这个概念在区块链中非常有意义,上链的区块不一定最终安全,在P2P网络上来讲,最终安全理论上来讲包含两个要素，1.此区块已经被绝大多数P2P节点确认,2.此区块在链上已经经过一段时间(有子区块的产生)),对于Final区块来讲，表示他已经通过共识机制来写入区块链中。
        //这里还会加载transaction index来加速对区块链的查询,默认仅对最近235w个区块进行加速查询
	eth.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, config.Genesis, &overrides, eth.engine, vmConfig, &config.TransactionHistory)
	if err != nil {
		return nil, err
	}
	eth.bloomIndexer.Start(eth.blockchain)
        // EIP4844通过了一种新的交易数据结构blob来扩大以太坊中的数据存储大小,这有利于layer2的gas费进一步降低,对于底层来讲,这被抽象成一种新的交易Pool给予txPool进行集成。
	if config.BlobPool.Datadir != "" {
		config.BlobPool.Datadir = stack.ResolvePath(config.BlobPool.Datadir)
	}
	blobPool := blobpool.New(config.BlobPool, eth.blockchain)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = stack.ResolvePath(config.TxPool.Journal)
	}
	legacyPool := legacypool.New(config.TxPool, eth.blockchain)
        //这里的txpool.New会开启一个协程对区块进行监控，防止被区块的交易依然保留在交易池里
	eth.txPool, err = txpool.New(config.TxPool.PriceLimit, eth.blockchain, []txpool.SubPool{legacyPool, blobPool})
	if err != nil {
		return nil, err
	}
	// Permit the downloader to use the trie cache allowance during fast sync
	cacheLimit := cacheConfig.TrieCleanLimit + cacheConfig.TrieDirtyLimit + cacheConfig.SnapshotLimit
        //开启downloader对区块链历史数据进行同步
	if eth.handler, err = newHandler(&handlerConfig{
		NodeID:         eth.p2pServer.Self().ID(),
		Database:       chainDb,
		Chain:          eth.blockchain,
		TxPool:         eth.txPool,
		Network:        networkID,
		Sync:           config.SyncMode,
		BloomCache:     uint64(cacheLimit),
		EventMux:       eth.eventMux,
		RequiredBlocks: config.RequiredBlocks,
	}); err != nil {
		return nil, err
	}
      
	eth.miner = miner.New(eth, config.Miner, eth.engine)
	eth.miner.SetExtra(makeExtraData(config.Miner.ExtraData))

	eth.APIBackend = &EthAPIBackend{stack.Config().ExtRPCEnabled(), stack.Config().AllowUnprotectedTxs, eth, nil}
	if eth.APIBackend.allowUnprotectedTxs {
		log.Info("Unprotected transactions allowed")
	}
	eth.APIBackend.gpo = gasprice.NewOracle(eth.APIBackend, config.GPO, config.Miner.GasPrice)

	// Start the RPC service
	eth.netRPCService = ethapi.NewNetAPI(eth.p2pServer, networkID)

	// 注册eth APIS方法里面的相关接口到rpcAPIS
	stack.RegisterAPIs(eth.APIs())
	stack.RegisterProtocols(eth.Protocols())
        //注册eth到lifecycle
	stack.RegisterLifecycle(eth)

	// Successful startup; push a marker and check previous unclean shutdowns.
	eth.shutdownTracker.MarkStartup()

	return eth, nil
}
```

上述会对本地节点的区块链数据进行构建,区块链节点为了降低索引数据过大，将整个区块链数据以类似冷热数据的方式分割成两种存储,对于第一种基于LSM的KV的数据库,只保存前9w个区块的索引，剩下的会以另一种freezer方式存储(通过idx文件以6字节为一个单位保存对内容的索引,对每个内容文件以最大为2GB进行切割，具体的内容包括区块信息(receipt、header和body),以及状态信息(account、history和storage)),如果想详细了解freezer这种存储特性请参考下面两篇文章。
[ethereum-internals-geth-storage-model](https://ferranbt.com/posts/ethereum-internals-geth-storage-model)
[geth-freezer-files-block-data-done-fast](https://superlunar.com/post/geth-freezer-files-block-data-done-fast)

同时,geth会对当前节点的区块链数据进行验证和构建,构建出账户状态以及区块链的状态来为区块链的运行做准备。在后续geth会构建交易池来对用户的交易进行统一的管理,同时会开始异步协程对已经上链的交易进行监控并消除，与此同时，geth通过newHandler()对历史数据进行同步，这里有一个细节的点在于当bc.currentHeader的Number为0时,代表区块链还未同步完成,这不代表目前的block的高度为0。

> 对于ethereum来讲,对于状态的存储通常都以MPT树的方式进行，但是在后续的改进中，为了便于对于状态的快速验证，后续推出了verkle的数据结构来加速状态的验证，如果你对verkle感兴趣的话，下面有一些作者认为比较专业的文章对此进行描述，这里不会做过多剖析，后续会单独推出一篇文章对此进行详细的剖析。[Verkle 树](https://learnblockchain.cn/article/2684) ,[PCS multiproofs using random evaluation](https://dankradfeist.de/ethereum/2021/06/18/pcs-multiproofs.html),[verkle_tree_eip](https://notes.ethereum.org/@vbuterin/verkle_tree_eip)


在以太坊中对于区块有多种的状态，这主要是为了在去中心化系统下保证交易的最终确定性，这在很多layer2的链里面也有类似的设计，详细可以参考[pos共识](https://ethereum.org/zh/developers/docs/consensus-mechanisms/pos/)

最后来到我们关注的点，这里会通过stack.RegisterAPIs(eth.APIs())、stack.RegisterLifecycle(eth)来对我们上述关注的两个点进行修改。

> 这里回顾一下，上述我们通过对startNode进行剖析，了解到ethereum会注册rpcAPIS的相关方法来为外部和内部提供接口式的服务,同时会遍历lifecycles,对每一个元素调用start方法


```go
// APIs return the collection of RPC services the ethereum package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Ethereum) APIs() []rpc.API {
	apis := ethapi.GetAPIs(s.APIBackend)

	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "miner",
			Service:   NewMinerAPI(s),
		}, {
			Namespace: "eth",
			Service:   downloader.NewDownloaderAPI(s.handler.downloader, s.blockchain, s.eventMux),
		}, {
			Namespace: "admin",
			Service:   NewAdminAPI(s),
		}, {
			Namespace: "debug",
			Service:   NewDebugAPI(s),
		}, {
			Namespace: "net",
			Service:   s.netRPCService,
		},
	}...)
}

```

```go
func GetAPIs(apiBackend Backend) []rpc.API {
	nonceLock := new(AddrLocker)
	return []rpc.API{
		{
			Namespace: "eth",
			Service:   NewEthereumAPI(apiBackend),
		}, {
			Namespace: "eth",
			Service:   NewBlockChainAPI(apiBackend),
		}, {
			Namespace: "eth",
			Service:   NewTransactionAPI(apiBackend, nonceLock),
		}, {
			Namespace: "txpool",
			Service:   NewTxPoolAPI(apiBackend),
		}, {
			Namespace: "debug",
			Service:   NewDebugAPI(apiBackend),
		}, {
			Namespace: "eth",
			Service:   NewEthereumAccountAPI(apiBackend.AccountManager()),
		},
	}
}
```

上述的接口主要是提供对ethereum blockchain的服务调用，包括发送交易，获取区块链信息等等,注册的这些方法都能在[ethereum的JSON-RPC文档](https://ethereum.github.io/execution-apis/api-documentation/),这是一个好消息，意味着我们找到了外部访问以太坊的所有入口(这些方法都会后续以单独的文章为各位读者剖析)。

接下来让我们关注stack.RegisterLifecycle(eth)中eth的start方法



```go
// Start implements node.Lifecycle, starting all internal goroutines needed by the
// Ethereum protocol implementation.
func (s *Ethereum) Start() error {
	s.setupDiscovery()

	// Start the bloom bits servicing goroutines
	s.startBloomHandlers(params.BloomBitsBlocks)

	// Regularly update shutdown marker
	s.shutdownTracker.Start()

	// Start the networking layer
	s.handler.Start(s.p2pServer.MaxPeers)
	return nil
}
```


```go
func (h *handler) Start(maxPeers int) {
	h.maxPeers = maxPeers

	// broadcast and announce transactions (only new ones, not resurrected ones)
	h.wg.Add(1)
	h.txsCh = make(chan core.NewTxsEvent, txChanSize)
	h.txsSub = h.txpool.SubscribeTransactions(h.txsCh, false)
        //以太坊会对用户签名的交易进行广播以便让交易更快上链
	go h.txBroadcastLoop()

	// 对获取到的transaction进行处理，对于P2P系统来说，可能会收到重复的消息以及从未收到必要收到的消息,以太坊通过包装txFetcher对上述情况进行处理,简单来讲，它负责根据公告获取新的交易。整个过程可以分为三个阶段，并且确保了数据的完整性和避免资源的浪费。每笔交易在txFetcher中会经过三个阶段
       //交易等待阶段：
//新发现的交易会被移入一个等待列表（wait list）。这个阶段是交易还未完全获取到的数据状态。
      // 交易排队阶段：
//大约 500 毫秒后，如果等待列表中的交易没有完全从网络中广播给节点，它们将会被移入一个排队区域（queueing area）。这意味着在这段时间内，如果某个交易还没有完全获取到，系统会准备好进一步处理这些交易。
     //交易获取阶段：
//当一个连接的对等节点没有任何正在进行的获取请求时，排队区域中的交易会分配给该对等节点，进入获取状态（fetching）。接下来，交易会被发送请求获取，直到交易被成功获取或获取失败。
	h.txFetcher.Start()

	// start peer handler tracker
	h.wg.Add(1)
	go h.protoTracker()
}
```


## 总结


<!--StartFragment-->

![geth结构体](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/geth%E7%BB%93%E6%9E%84%E4%BD%93.png)


总的来说,geth通过makeFullNode和startNode对上述的一些服务进行了初始化以及启动。主要包括p2pNode、blockchain这个复杂的数据结构，以及向外部和内部进程提供的服务servers(**通过反射rpcAPIS的servers里面的公共方法实现注册**),同时通过accountmanager对账户进行管理,后续包括了一个系统必不可少了profile、metrics、trace等相关信息。这里我们没有过多地讲述consensus engine，因为目前很多full node都是通过[第三方的共识模块](https://ethereum.org/zh/developers/docs/nodes-and-clients/run-a-node/#starting-the-consensus-client)来实现POS的。

geth对各个组件进行初始化来完成这个复杂系统的协同性工作,这里面包含了许多复杂原理和数学知识，这篇文章粗略讲解full node的整体服务。我们在开篇讲过，我们从geth给外界提供的接口出发来深入剖析每一步的流程。