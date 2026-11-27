package i18n

// The strings of MCP across machines: TLS for MCP over HTTP (`bowel mcp
// token`, `bowel mcp cert`) and the guard rails of the gateway's and the
// providers' tools, kept apart so that work on other parts merges cleanly.

func init() {
	for k, v := range jaMCPRemote {
		jaTable[k] = v
	}
	for k, v := range hantMCPRemote {
		hantTable[k] = v
	}
}

var jaMCPRemote = map[string]string{
	"未知的 mcp 子命令 %s；可以用 bowel mcp、bowel mcp token 或 bowel mcp cert": "不明な mcp サブコマンドです: %s。bowel mcp、bowel mcp token、bowel mcp cert のどれかを使ってください",
	"bowel mcp token 不认识 %s；可以用 -q 和 --name <名字>":                   "bowel mcp token は %s を受け付けません。使えるのは -q と --name <名前> です",
	"服务器名 %q 只能用字母、数字、- 和 _":                                        "サーバー名 %q に使えるのは英字、数字、- と _ だけです",
	"令牌来自 %s。从别的机器连接时，把 localhost 换成这台机器的地址；令牌以明文经过网络，跨网络请放在 TLS 或 VPN 后面（%s=on 开启 TLS）。": "トークンの出どころ: %s。別のマシンから接続するときは localhost をこのマシンのアドレスに置き換えてください。トークンは平文でネットワークを通るため、ネットワークをまたぐ場合は TLS か VPN の内側に置いてください（%s=on で TLS がオンになります）。",
	"自签名证书": "自己署名証明書",
	"证书":    "証明書",
	"客户端要信任这张证书：在这台机器上执行 bowel mcp cert > %[1]s.pem，把文件拷到 agent 所在的机器，Claude Code 用 NODE_EXTRA_CA_CERTS=%[1]s.pem 启动（有几台 bowel 就把几个证书拼进同一个文件）。第一次连接前核对指纹：\n  openssl s_client -connect <地址>:%[2]s </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256\n": "クライアントはこの証明書を信頼する必要があります。このマシンで bowel mcp cert > %[1]s.pem を実行し、ファイルをエージェントのマシンにコピーして、Claude Code を NODE_EXTRA_CA_CERTS=%[1]s.pem 付きで起動してください（bowel が複数あるときは、すべての証明書を同じ一つのファイルにまとめます）。最初に接続する前にフィンガープリントを確認してください:\n  openssl s_client -connect <アドレス>:%[2]s </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256\n",
	"从别的机器连接时，把 localhost 换成证书里有的名字或地址（%s 可以加）。令牌来自 %s。\n": "別のマシンから接続するときは、localhost を証明書に含まれる名前かアドレスに置き換えてください（%s で追加できます）。トークンの出どころ: %s。\n",
	"从别的机器连接时，把 localhost 换成证书上的名字。令牌来自 %s。\n":             "別のマシンから接続するときは、localhost を証明書の名前に置き換えてください。トークンの出どころ: %s。\n",
	"MCP 的 HTTP 服务没有用 TLS：启动 bowel 时设置 %s=:%s 和 %s=on":     "MCP の HTTP サーバーは TLS を使っていません。bowel を %s=:%s と %s=on を付けて起動してください",
	"通过 HTTP 连接时不能添加 bowel 所在机器上的文件：请用订阅链接或节点链接（source）":   "HTTP 経由の接続では、bowel が動いているマシン上のファイルは追加できません。サブスクリプションの URL かノードのリンク（source）を使ってください",
	"bowel 正在给局域网分配地址（自动，%d 个租约）：现在关闭网关，从这里领到地址的设备都会没有路由器、断网。先由用户在路由器的管理页面重新打开它的 DHCP（这一步是用户的），再用 set_gateway_auto 退回手动（它会先检测），等 list_devices 里不再有在线的 auto 设备，再关闭网关。用户听过这些仍要现在关闭，才传 force: true": "bowel が LAN のアドレスを配っています（自動、リース %d 件）。いまゲートウェイをオフにすると、ここでアドレスを受け取ったデバイスはすべてルーターを失いオフラインになります。まずユーザーがルーターの管理画面で DHCP をオンに戻し（これはユーザーの作業です）、次に set_gateway_auto で手動に戻し（先に確認します）、list_devices にオンラインの auto デバイスがなくなってからゲートウェイをオフにしてください。それを聞いたうえでユーザーがいますぐオフにしたい場合だけ force: true を渡してください",
	"有 %d 台设备正经过这台电脑上网（%s）：关闭网关后，它们要等各自改回原来的路由器（%s）才能上网，这要用户在每台设备上改。先告诉用户是哪些设备；用户同意后再传 force: true":                                                                                              "%d 台のデバイスがいまこのマシン経由で通信しています（%s）。ゲートウェイをオフにすると、それぞれが元のルーター（%s）に戻されるまで通信できません。これはユーザーが各デバイスで行います。どのデバイスかをユーザーに伝え、同意を得てから force: true を渡してください",
	"没法确认路由器的 DHCP 是否已重新打开：%s。退回手动后，从这里领到地址的设备可能断网；用户确认路由器的 DHCP 已打开后，再传 force: true":                                                                                                            "ルーターの DHCP がオンに戻ったか確認できませんでした: %s。手動に戻すと、ここでアドレスを受け取ったデバイスがオフラインになるかもしれません。ルーターの DHCP がオンだとユーザーが確認したら force: true を渡してください",
	"局域网里没有别的 DHCP 服务器在应答：现在退回手动，从这里领到地址的设备（%d 个租约）续不了租，会断网。先由用户在路由器的管理页面重新打开它的 DHCP（这一步是用户的），然后再调用一次：它会先检测。只有用户听过这些仍要退回（比如已经没有路由器可回），才传 force: true":                                            "LAN でほかの DHCP サーバーが応答していません。いま手動に戻すと、ここでアドレスを受け取ったデバイス（リース %d 件）は更新できずオフラインになります。まずユーザーがルーターの管理画面で DHCP をオンに戻し（これはユーザーの作業です）、それからもう一度呼んでください。先に確認します。それを聞いたうえでユーザーが手動に戻したい場合（戻るルーターがもうない、など）だけ force: true を渡してください",
}

var hantMCPRemote = map[string]string{
	"未知的 mcp 子命令 %s；可以用 bowel mcp、bowel mcp token 或 bowel mcp cert": "未知的 mcp 子指令 %s；可以用 bowel mcp、bowel mcp token 或 bowel mcp cert",
	"bowel mcp token 不认识 %s；可以用 -q 和 --name <名字>":                   "bowel mcp token 不認得 %s；可以用 -q 和 --name <名稱>",
	"服务器名 %q 只能用字母、数字、- 和 _":                                        "伺服器名稱 %q 只能用字母、數字、- 和 _",
	"令牌来自 %s。从别的机器连接时，把 localhost 换成这台机器的地址；令牌以明文经过网络，跨网络请放在 TLS 或 VPN 后面（%s=on 开启 TLS）。": "權杖來自 %s。從別的機器連線時，把 localhost 換成這台機器的位址；權杖以明文經過網路，跨網路請放在 TLS 或 VPN 後面（%s=on 開啟 TLS）。",
	"自签名证书": "自簽憑證",
	"证书":    "憑證",
	"客户端要信任这张证书：在这台机器上执行 bowel mcp cert > %[1]s.pem，把文件拷到 agent 所在的机器，Claude Code 用 NODE_EXTRA_CA_CERTS=%[1]s.pem 启动（有几台 bowel 就把几个证书拼进同一个文件）。第一次连接前核对指纹：\n  openssl s_client -connect <地址>:%[2]s </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256\n": "用戶端要信任這張憑證：在這台機器上執行 bowel mcp cert > %[1]s.pem，把檔案複製到 agent 所在的機器，Claude Code 用 NODE_EXTRA_CA_CERTS=%[1]s.pem 啟動（有幾台 bowel 就把幾張憑證併進同一個檔案）。第一次連線前核對指紋：\n  openssl s_client -connect <位址>:%[2]s </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256\n",
	"从别的机器连接时，把 localhost 换成证书里有的名字或地址（%s 可以加）。令牌来自 %s。\n": "從別的機器連線時，把 localhost 換成憑證裡有的名稱或位址（%s 可以加）。權杖來自 %s。\n",
	"从别的机器连接时，把 localhost 换成证书上的名字。令牌来自 %s。\n":             "從別的機器連線時，把 localhost 換成憑證上的名稱。權杖來自 %s。\n",
	"MCP 的 HTTP 服务没有用 TLS：启动 bowel 时设置 %s=:%s 和 %s=on":     "MCP 的 HTTP 服務沒有用 TLS：啟動 bowel 時設定 %s=:%s 和 %s=on",
	"通过 HTTP 连接时不能添加 bowel 所在机器上的文件：请用订阅链接或节点链接（source）":   "透過 HTTP 連線時不能加入 bowel 所在機器上的檔案：請用訂閱連結或節點連結（source）",
	"bowel 正在给局域网分配地址（自动，%d 个租约）：现在关闭网关，从这里领到地址的设备都会没有路由器、断网。先由用户在路由器的管理页面重新打开它的 DHCP（这一步是用户的），再用 set_gateway_auto 退回手动（它会先检测），等 list_devices 里不再有在线的 auto 设备，再关闭网关。用户听过这些仍要现在关闭，才传 force: true": "bowel 正在為區域網路分配位址（自動，%d 個租約）：現在關閉閘道，從這裡領到位址的裝置都會沒有路由器、斷線。先由使用者在路由器的管理頁面重新開啟它的 DHCP（這一步是使用者的），再用 set_gateway_auto 退回手動（它會先偵測），等 list_devices 裡不再有在線的 auto 裝置，再關閉閘道。使用者聽過這些仍要現在關閉，才傳 force: true",
	"有 %d 台设备正经过这台电脑上网（%s）：关闭网关后，它们要等各自改回原来的路由器（%s）才能上网，这要用户在每台设备上改。先告诉用户是哪些设备；用户同意后再传 force: true":                                                                                              "有 %d 台裝置正經過這台電腦上網（%s）：關閉閘道後，它們要等各自改回原來的路由器（%s）才能上網，這要使用者在每台裝置上改。先告訴使用者是哪些裝置；使用者同意後再傳 force: true",
	"没法确认路由器的 DHCP 是否已重新打开：%s。退回手动后，从这里领到地址的设备可能断网；用户确认路由器的 DHCP 已打开后，再传 force: true":                                                                                                            "無法確認路由器的 DHCP 是否已重新開啟：%s。退回手動後，從這裡領到位址的裝置可能斷線；使用者確認路由器的 DHCP 已開啟後，再傳 force: true",
	"局域网里没有别的 DHCP 服务器在应答：现在退回手动，从这里领到地址的设备（%d 个租约）续不了租，会断网。先由用户在路由器的管理页面重新打开它的 DHCP（这一步是用户的），然后再调用一次：它会先检测。只有用户听过这些仍要退回（比如已经没有路由器可回），才传 force: true":                                            "區域網路裡沒有別的 DHCP 伺服器在回應：現在退回手動，從這裡領到位址的裝置（%d 個租約）續不了約，會斷線。先由使用者在路由器的管理頁面重新開啟它的 DHCP（這一步是使用者的），然後再呼叫一次：它會先偵測。只有使用者聽過這些仍要退回（例如已經沒有路由器可回），才傳 force: true",
}
