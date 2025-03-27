



> 以上为作者从零开始对Ethereum源码进行剖析，在目前绝大多数的资料中，只是对ethereum的架构做了一个科普性的介绍，没有对ethereum的实现做过多阐述。作为开发者而言，了解web3的前提是必须熟悉一条区块链的底层实现，因为他代表了一种P2P的新范式,这与以往web2的开发模式大相径庭，这更多的是一种理念的创新。不管是Ethereum还是Bitcoin或者说是其他的Layer2以及Solana,其最终都会遵守一个最基本的开发理念:去中心化。作者想通过对Ethereum源码的分析来对当前的所有链的底层原理有一个通俗的理解,同时想借用这系列的文章与更多web3的工作者交流技术。[源码版本下载](https://github.com/0xdoomxy/go-ethereum)

[作者博客原文地址](https://www.0xdoomxy.top/#/article/4)
## Geth的构建

在[Go-Ethereum的Github仓库](https://github.com/ethereum/go-ethereum)中，我们很清楚的看到一个Ethereum节点从 make geth 开始。

![make_geth](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/make%20geth.png)



在Makefile文件中,make geth的命令被解析为 go run  build/ci.go install ./cmd/geth

```shell

geth:
	$(GORUN) build/ci.go install ./cmd/geth
	@echo "Done building."
	@echo "Run \"$(GOBIN)/geth\" to launch geth."


```

在此不对build/ci.go的执行做任何讲解(他仅仅是为了构建geth可执行程序而存在的，不影响geth的运行),其最终会通过MustRun来开启一个新的进程来build ./cmd/geth文件，相当于替换为 go build  ./cmd/geth。

![geth_build](hhttps://github.com/0xdoomxy/web3/blob/main/images/chapter1/geth%20build.png)



## Geth运行Ethereum Full Node的核心


!![geth_console](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/geth%20console.png)


在[Go-Ethereum的Github仓库](https://github.com/ethereum/go-ethereum)的文档中，通过geth console来运行一个以太坊主网的全节点,通过上面对Geth的构建剖析,我们可以直接替换为 go run ./cmd/geth/main.go console 来理解ethereum full node如何工作。

```go

// geth 会在初始化的时候构建默认的命令行的指令,与著名的cobra框架有相同的设计模式,
//这里我们需要关注Action这个字段，这个代表相关的执行流程。
func init() {
	app.Action = geth
        // 这是解析相关子命令触发的geth的行为,这里根据我们传入的参数来看geth console,
       // 我们仅关注consule command,在阅读源码的过程中，先看主干，后再阅读其他扩展功能
       // 避免刚阅读源码时因主次把握不清晰导致的思维混乱。
	app.Commands = []*cli.Command{
		// See chaincmd.go:
		initCommand,
		importCommand,
		exportCommand,
		importHistoryCommand,
		exportHistoryCommand,
		importPreimagesCommand,
		removedbCommand,
		dumpCommand,
		dumpGenesisCommand,
		// See accountcmd.go:
		accountCommand,
		walletCommand,
		// See consolecmd.go:
		consoleCommand,
		attachCommand,
		javascriptCommand,
		// See misccmd.go:
		versionCommand,
		versionCheckCommand,
		licenseCommand,
		// See config.go
		dumpConfigCommand,
		// see dbcmd.go
		dbCommand,
		// See cmd/utils/flags_legacy.go
		utils.ShowDeprecated,
		// See snapshot.go
		snapshotCommand,
		// See verkle.go
		verkleCommand,
	}
	if logTestCommand != nil {
		app.Commands = append(app.Commands, logTestCommand)
	}
	sort.Sort(cli.CommandsByName(app.Commands))

	app.Flags = slices.Concat(
		nodeFlags,
		rpcFlags,
		consoleFlags,
		debug.Flags,
		metricsFlags,
	)
	flags.AutoEnvVars(app.Flags, "GETH")
        //这里是建立的两个钩子函数，用于加强Action函数的作用,不会影响到geth的核心流程
        //会对相关 GMP中P个数进行设置同时控制日志和性能测试相关事件等等。
	app.Before = func(ctx *cli.Context) error {
		maxprocs.Set() // Automatically set GOMAXPROCS to match Linux container CPU quota.
		flags.MigrateGlobalFlags(ctx)
		if err := debug.Setup(ctx); err != nil {
			return err
		}
                
		flags.CheckEnvVars(ctx, app.Flags, "GETH")
		return nil
	}
        // 在命令结束后对资源的释放
	app.After = func(ctx *cli.Context) error {
		debug.Exit()
		prompt.Stdin.Close() // Resets terminal mode.
		return nil
	}
}
//go run ./cmd/geth/main.go console的主入口
func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

```

在上面的init方法和main方法中,最重要的是对app这个变量进行初始化后执行Run方法，并且携带了用户传入的参数,下面来看看Run方法的核心流程

> 如果不了解cobra,可以通过下面文章了解[万字长文——Go 语言现代命令行框架 Cobra 详解](https://zhuanlan.zhihu.com/p/627848739)

```go

// 这里调用了RunContext子方法并且附带了一个上下文(可能是会用这个上下文做超时控制或者保存变量)
func (a *App) Run(arguments []string) (err error) {
	return a.RunContext(context.Background(), arguments)
}

func (a *App) RunContext(ctx context.Context, arguments []string) (err error) {

	//运行前的状态检查，判断是否符合运行条件
	a.Setup()
	// 检查是否开启参数的自动补全
	shellComplete, arguments := checkShellCompleteFlag(a, arguments)
	//创建应用级上下文
	cCtx := NewContext(a, nil, &Context{Context: ctx})
	cCtx.shellComplete = shellComplete

	a.rootCommand = a.newRootCommand()
	cCtx.Command = a.rootCommand
	//核心逻辑
	return a.rootCommand.Run(cCtx, arguments...)
}

```



![root_command](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/root%20command.png)

上面会初始化应用上下文，执行相关检查工作，并执行Run方法,从newRootCommand的方法来看rootCommand其实代表了App的相关行为,这里我们重点关注rootCommand的Action是被赋予了a.Action也就是在init方法中讲到的geth,下面我们来解析rootCommand.Run方法


```go

func (c *Command) Run(cCtx *Context, arguments ...string) (err error) {
        //只会在顶层Command的时候初始化ctx,即初始化一次。
	if !c.isRoot {
		c.setup(cCtx)
	}
        //下面所有操作即在执行真正方法之前进行check,以保证最终的方法能够正常执行,
        //这里不需要过多关注check相关的方法
	a := args(arguments)
        //解析用户传入的参数,并尝试更改command的行为,这会在下面体现
        //这里的函数会将我们参数中的console command提取出来，因为这个操作代表最终的geth行为,请注意这个flagSet
	set, err := c.parseFlags(&a, cCtx.shellComplete)
	cCtx.flagSet = set

	if checkCompletions(cCtx) {
		return nil
	}

	if err != nil {
		if c.OnUsageError != nil {
			err = c.OnUsageError(cCtx, err, !c.isRoot)
			cCtx.App.handleExitCoder(cCtx, err)
			return err
		}
		_, _ = fmt.Fprintf(cCtx.App.Writer, "%s %s\n\n", "Incorrect Usage:", err.Error())
		if cCtx.App.Suggest {
			if suggestion, err := c.suggestFlagFromError(err, ""); err == nil {
				fmt.Fprintf(cCtx.App.Writer, "%s", suggestion)
			}
		}
		if !c.HideHelp {
			if c.isRoot {
				_ = ShowAppHelp(cCtx)
			} else {
				_ = ShowCommandHelp(cCtx.parentContext, c.Name)
			}
		}
		return err
	}

	if checkHelp(cCtx) {
		return helpCommand.Action(cCtx)
	}

	if c.isRoot && !cCtx.App.HideVersion && checkVersion(cCtx) {
		ShowVersion(cCtx)
		return nil
	}
         
	if c.After != nil && !cCtx.shellComplete {
		defer func() {
			afterErr := c.After(cCtx)
			if afterErr != nil {
				cCtx.App.handleExitCoder(cCtx, err)
				if err != nil {
					err = newMultiError(err, afterErr)
				} else {
					err = afterErr
				}
			}
		}()
	}

	cerr := cCtx.checkRequiredFlags(c.Flags)
	if cerr != nil {
		_ = helpCommand.Action(cCtx)
		return cerr
	}

	if c.Before != nil && !cCtx.shellComplete {
		beforeErr := c.Before(cCtx)
		if beforeErr != nil {
			cCtx.App.handleExitCoder(cCtx, beforeErr)
			err = beforeErr
			return err
		}
	}
        //对于c.Flags如果有需要执行的Action,则直接执行
	if err = runFlagActions(cCtx, c.Flags); err != nil {
		return err
	}

	var cmd *Command
        //这里会获取之前设置的flagSet,如果用户没有传入command,那么geth会gethCommand作为此次的行为
	args := cCtx.Args()
	if args.Present() {
           //会获取到console,并从app.Commands里面匹配console的command,并改变原来的geth的行为
		name := args.First()
		cmd = c.Command(name)
             //如果未匹配到，则调用默认的App.DefaultCommand。这里的DefaultCommand一般为空
		if cmd == nil {
			hasDefault := cCtx.App.DefaultCommand != ""
			isFlagName := checkStringSliceIncludes(name, cCtx.FlagNames())

			var (
				isDefaultSubcommand   = false
				defaultHasSubcommands = false
			)

			if hasDefault {
				dc := cCtx.App.Command(cCtx.App.DefaultCommand)
				defaultHasSubcommands = len(dc.Subcommands) > 0
				for _, dcSub := range dc.Subcommands {
					if checkStringSliceIncludes(name, dcSub.Names()) {
						isDefaultSubcommand = true
						break
					}
				}
			}

			if isFlagName || (hasDefault && (defaultHasSubcommands && isDefaultSubcommand)) {
				argsWithDefault := cCtx.App.argsWithDefaultCommand(args)
				if !reflect.DeepEqual(args, argsWithDefault) {
					cmd = cCtx.App.rootCommand.Command(argsWithDefault.First())
				}
			}
		}
	} else if c.isRoot && cCtx.App.DefaultCommand != "" {
		if dc := cCtx.App.Command(cCtx.App.DefaultCommand); dc != c {
			cmd = dc
		}
	}
        //如果原有的command被更改了行为，则更新ctx上下文并重新执行Run方法
	if cmd != nil {
		newcCtx := NewContext(cCtx.App, nil, cCtx)
		newcCtx.Command = cmd
		return cmd.Run(newcCtx, cCtx.Args().Slice()...)
	}

	if c.Action == nil {
		c.Action = helpCommand.Action
	}
        // 核心逻辑,调用Command的Action方法来执行用户想要的最终程序，这里的是执行ConsoleCommand的Action方法
	err = c.Action(cCtx)
        
	cCtx.App.handleExitCoder(cCtx, err)
	return err
}

```
![console_command](https://github.com/0xdoomxy/web3/blob/main/images/chapter1/console%20command.png)


这里主要是对于当前命令行参数的一个处理非常复杂,对于geth来说,需要阅读用户的参数来执行最终的行为，默认会执行gethCommand,当我们传入console参数，geth会识别consoleCommand来作为我们最终赋予geth的行为,并且生成ctx来作为此次命令的上下文，从上面结果来看，geth执行了consoleCommand的localConsole方法，并且传入了cCtx上下文。



## 总结

执行geth console的时候,最终会转化为调用consoleCommand的Action属性的方法，并且传入cCtx来运行geth。在此之前geth会解析环境变量以及命令行参数,geth的参数不仅涉及日志、指标、性能报告、数据存储、身份认证等系统信息的设置,而且包含p2p、miner、gasprice、blobpool以及transaction pool、beacon chian等此类ethereum运行参数的设置。


## 参考资料

+ [go-ethereum](https://github.com/ethereum/go-ethereum)
+ [beacon-chain](https://ethos.dev/beacon-chain)
+ [cobra](https://github.com/spf13/cobra)
+ [eip-4844](https://learnblockchain.cn/article/7586)

