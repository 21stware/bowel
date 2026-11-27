<p align="center">
  <img src="site/icon.png" width="112" alt="bowel">
</p>

<h1 align="center">bowel</h1>

<p align="center">
bowel; macOS, Windows, Linux ve Android için <a href="https://github.com/MetaCubeX/mihomo">mihomo</a> (Clash Meta) tabanlı bir istemcidir: Electron veya WebView değil, yerel (native) bir uygulamadır.
</p>

<p align="center">
  <a href="https://github.com/21stware/bowel/releases/latest"><b>İndir</b></a> ·
  <a href="https://bowel.dev/en/">Web Sitesi</a> ·
  <a href="https://bowel.dev/en/docs/">Belgeler</a> ·
  <a href="https://t.me/+BI3KVgBn7SE3YjVh">Telegram</a> ·
  <a href="README.md">English</a> ·
  <a href="README.zh-CN.md">简体中文</a>
</p>

> [!NOTE]
> Bu sayfa topluluk tarafından çevrilmiştir ve 27 Kasım 2026 tarihli İngilizce sürüme dayanır; sonraki değişiklikler buraya yansımamış olabilir. Güncel ve geçerli sürüm için [İngilizce README](README.md)'ye bakın.

<p align="center">
  <a href="https://github.com/21stware/bowel/releases/latest"><img src="https://img.shields.io/github/v/release/21stware/bowel?label=release" alt="Son sürüm"></a>
  <a href="https://github.com/21stware/bowel/releases"><img src="https://img.shields.io/github/downloads/21stware/bowel/total?label=downloads" alt="İndirmeler"></a>
</p>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="site/shots/home-en-dark.webp">
    <img src="site/shots/home-en.webp" width="880" alt="bowel ana penceresi">
  </picture>
</p>

Masaüstü arayüzü Go ile geliştirilmiştir ve WebView olmadan doğrudan GPU üzerinden çizilir. Android uygulaması Flutter ile geliştirilmiştir ve aynı kartlara ve temalara sahiptir.

## Farkı ne

- **Her amaç için ayrı bir düğüm (node).** Yapay zeka servisleri, yayın akışı, geliştirme ve diğerleri ayrı rotalardır; her birinin kendi düğümü vardır ve elle ya da hıza göre seçilir.
- **Clash yapılandırmaları olduğu gibi.** Abonelik bağlantıları, düğüm bağlantıları, QR kodları ve kendi gruplarını ve kurallarını koruyabilen tam Clash veya mihomo yapılandırmaları.
- **Geliştirici makineleri için tasarlandı.** macOS ve Linux'ta Docker, terminal ve Git tek tıkla ayarlanır; npm, pip, Go ve Cargo yansıları (mirror) ölçülür ve değiştirilir; yerel ağ (LAN), Tailscale ve şirket VPN'leri doğrudan geçer.
- **Bir komut satırı, bir TUI ve bir arka plan hizmeti (daemon).** `bowel` çalışan uygulamayı yönetir, betikler için `--json` desteği sunar; `bowel tui` SSH üzerinden çalışır; `bowel daemon` penceresiz çalışır.
- **Ajanlar için bir MCP sunucusu.** Claude Code ve diğer ajanlar durumu okuyup bağlantı sorunlarını giderir ve bir şeyi değiştirmeden önce size sorar.
- **Ağ geçidi (Gateway) modu.** Tek bir Mac veya Linux bilgisayar yerel ağdaki (LAN) TV'leri ve konsolları taşır.

## Özellikler

Ekran görüntüleri macOS'tendir.

### Rotalar
Trafiği amaca göre yönlendirin ve çıkış düğümlerini elle veya hıza göre otomatik seçin. Grupların, test sitelerinin ve indirme testinin nasıl çalıştığı: [Rota grupları ve düğümler](https://bowel.dev/en/docs/routes/).

<img src="site/shots/routes-en.webp" width="880" alt="Rotalar">

### Bağlantılar
Her bağlantı için uygulamayı, hedefi, kuralı, çıkışı ve hızı inceleyin veya alan adlarını anında yeniden yönlendirin.

<img src="site/shots/activity-en.webp" width="880" alt="Bağlantı günlüğü">

### Sağlayıcılar
Her kaynak için veri kullanımı ve son kullanma tarihi gösterilecek şekilde abonelik bağlantılarını, düğüm URL'lerini, Clash veya mihomo YAML yapılandırmalarını ya da QR kodlarını içe aktarın.

<img src="site/shots/providers-en.webp" width="880" alt="Sağlayıcılar">

### Menü çubuğu
Sistem vekil sunucusu (proxy) devralmasını açıp kapatın, vekil sunucu modlarını değiştirin ve hiçbir kuralın eşleşmediği trafik için düğümü menü çubuğundan ya da Windows ve Linux'ta sistem tepsisinden seçin. Omarchy üzerinde bowel simgesini çubuğun tepsisine sabitleyin, aksi halde tepsi çekmecesinde katlı kalır.

<img src="site/shots/panel-en.webp" width="300" alt="Menü çubuğu paneli">

### Temalar
Tüm arayüzün görünümünü değiştiren on altı tema.

<table>
  <tr>
    <td><img src="site/shots/skin-broadsheet-en.webp" alt="Broadsheet"><br><sub>Broadsheet</sub></td>
    <td><img src="site/shots/skin-phosphor-en.webp" alt="Phosphor"><br><sub>Phosphor</sub></td>
    <td><img src="site/shots/skin-neon-en.webp" alt="Neon"><br><sub>Neon</sub></td>
  </tr>
  <tr>
    <td><img src="site/shots/skin-blueprint-en.webp" alt="Blueprint"><br><sub>Blueprint</sub></td>
    <td><img src="site/shots/skin-sakura-en.webp" alt="Sakura"><br><sub>Sakura</sub></td>
    <td><img src="site/shots/skin-paper-en.webp" alt="Paper"><br><sub>Paper (varsayılan)</sub></td>
  </tr>
  <tr>
    <td><img src="site/shots/skin-shanshui-en.webp" alt="Shanshui"><br><sub>Shanshui</sub></td>
    <td><img src="site/shots/skin-celadon-en.webp" alt="Celadon"><br><sub>Celadon</sub></td>
    <td><img src="site/shots/skin-dunhuang-en.webp" alt="Dunhuang"><br><sub>Dunhuang</sub></td>
  </tr>
  <tr>
    <td><img src="site/shots/skin-dusk-en.webp" alt="Dusk"><br><sub>Dusk</sub></td>
    <td><img src="site/shots/skin-aurora-en.webp" alt="Aurora"><br><sub>Aurora</sub></td>
    <td><img src="site/shots/skin-obsidian-en.webp" alt="Obsidian"><br><sub>Obsidian</sub></td>
  </tr>
  <tr>
    <td><img src="site/shots/skin-riso-en.webp" alt="Riso"><br><sub>Riso</sub></td>
    <td><img src="site/shots/skin-mrrobot-en.webp" alt="Mr. Robot"><br><sub>Mr. Robot</sub></td>
    <td><img src="site/shots/skin-machine-en.webp" alt="The Machine"><br><sub>The Machine</sub></td>
  </tr>
  <tr>
    <td><img src="site/shots/skin-samaritan-en.webp" alt="Samaritan"><br><sub>Samaritan</sub></td>
  </tr>
</table>

### Diğer
- **Özel kurallar**: Belirli alan adlarını, anahtar kelimeleri, IP aralıklarını veya GeoSite ve GeoIP listelerini yönlendirin ve adreslerin nereye gittiğini test edin; mihomo sözdiziminde yapıştırılan bir kural da çalışır; macOS, Windows, Linux ve Android 10 veya üzeri sürümlerde uygulamaya göre de yönlendirme yapabilirsiniz: bir rota grubuna bir App kuralı ekleyin ve o uygulamanın tüm bağlantıları onun üzerinden geçsin.
- **Ölçülen çıkış konumları**: Cloudflare trace üzerinden gerçek çıkış konumlarını doğrulayın.
- **Ağ geçidi (Gateway) modu** (macOS, Linux): Bu bilgisayarı TV'ler ve konsollar gibi cihazlar için bir LAN ağ geçidine dönüştürün. İnsanlar ve ajanlar için tam kılavuz [docs/gateway.md](docs/gateway.md) dosyasındadır.
- **TUN** (macOS, Windows, Linux): Sistem vekil sunucusu ayarlarını yok sayan uygulamalardan gelen trafiği yakalayın.
- **Geliştirici araçları** (macOS, Linux): Docker, terminal ve Git sistem vekil sunucusunu yok sayar. bowel hangilerinin bağlı olmadığını listeler, kendi yapılandırmalarını tek tıkla yazar ve geri aldığınızda temiz bir şekilde kaldırır.
- **İndirme kaynakları**: npm, pip, Go ve Cargo için resmi kaynağı ve her yansıyı ölçer ve tek tıkla geçiş yapar. Mevcut kaynak yanıt vermeyi durdurduğunda, tek tıkla yanıt veren en hızlı kaynağa geçirir veya tekrar bir düğüm üzerinden gönderir.
- **Özel ağlar**: LAN, Tailscale ve şirket VPN'inizdeki adresler genel (global) modda da otomatik olarak doğrudan geçer.
- **Çıkış kilidi (Exit lock)**: AI gibi bir rota grubunu panodaki sayfasından kilitleyin: trafiği yalnızca bir düğüm üzerinden geçer ve hiçbir düğüm taşıyamadığında doğrudan gönderilmek yerine kesilir; bu kural global, direct modlarında ve devralma kapalıyken de geçerlidir. bowel kapandığında hiçbir şey engellenmez. `bowel route lock ai` aynı işi yapar. TUN ile ve TUN olmadan neleri tuttuğu ve Android üzerindeki davranışı: [Devralma, TUN ve çıkış kilidi](https://bowel.dev/en/docs/takeover/#exit-lock).
- **MCP**: Claude Code ve diğer ajanların durumu okuması ve bağlantı sorunlarını gidermesi için yerleşik MCP sunucusu. Varsayılan olarak ayarları değiştirmeden önce size sorar. Aşağıdaki komut macOS içindir; Windows ve Linux'ta bowel programının yolunu kullanın.
  ```bash
  claude mcp add --scope user bowel -- /Applications/bowel.app/Contents/MacOS/bowel mcp
  ```
- **Ayarlar ve diller**: Aranabilir ayarlar (⌘F); İngilizce, Basitleştirilmiş Çince, Geleneksel Çince ve Japonca; açık ve koyu tema.

## Kurulum

macOS, [Homebrew](https://brew.sh) ile:

```bash
brew install --cask 21stware/tap/bowel
```

Windows, PowerShell'de:

```powershell
irm https://github.com/21stware/bowel/releases/latest/download/install.ps1 | iex
```

Debian, Ubuntu ve Fedora. Bu işlem imzalı bir paket deposu ekler, böylece `apt` veya `dnf` bundan sonra bowel'ı günceller:

```bash
curl -fsSL https://21stware.github.io/packages/install.sh | sh
```

Ekranı olmayan bir sunucu, NAS veya yönlendirici, Docker'da (amd64, arm64, armv7): imaj `ghcr.io/21stware/bowel`; NAS arayüzlerinin arama yaptığı Docker Hub'da ise `oboochin/bowel`. Ardından `docker exec -it bowel bowel tui`; TUN, ağ geçidi modu, compose dosyaları, Synology ve Unraid ile ajanlar için MCP [docs/docker.md](docs/docker.md) dosyasındadır:

```bash
docker run -d --name bowel --restart unless-stopped -p 7899:7899 -v bowel:/data \
  -e BOWEL_SUBSCRIPTION='<subscription URL>' ghcr.io/21stware/bowel
```

Veya platformunuz için dosyayı [Releases](https://github.com/21stware/bowel/releases/latest) sayfasından indirin:

| Platform | Dosya | Gereksinim |
| --- | --- | --- |
| macOS | `bowel-<version>-arm64.dmg` | macOS 12 veya üzeri, Apple silicon |
| Windows | `bowel-setup-<version>-amd64.exe` veya `-arm64.exe` | x64 veya Arm64 |
| Linux | `bowel_<version>_<arch>.deb`, `bowel-<version>-1.<rpm arch>.rpm` veya `bowel-<version>-linux-<arch>.tar.gz` | amd64 veya arm64 |
| Android | `bowel-<version>-android-arm64.apk` | Android 8.0 veya üzeri, arm64 |

- **macOS**: bowel'ı Applications klasörüne sürükleyin. Uygulama bir Developer ID ile imzalanmış ve Apple tarafından noter onayından geçirilmiştir.
- **Windows**: Kurulum dosyasını çalıştırın. Kod imzalı değildir, bu nedenle Windows çalıştırmadan önce uyarı verebilir.
- **Linux**: `.deb` veya `.rpm` paketini kurun. `~/.local` içine kullanıcı kurulumu için aynı sürümden `install.sh` ve `.tar.gz` dosyalarını indirin, ardından arşivi açmadan `sh install.sh bowel-<version>-linux-<arch>.tar.gz` komutunu çalıştırın. Bu, yukarıdaki paket deposu betiğinden ayrıdır.
- **Android**: İstenirse tarayıcınızın uygulama yüklemesine izin vererek APK'yı kurun. Android ilk kez bağlandığınızda VPN izni ister.
- **iOS**: Henüz yayınlanmadı.

Ardından bir sağlayıcı (abonelik URL'si, düğüm bağlantıları veya QR kodu) ekleyin ve devralmayı açın: [Kurulum ve başlangıç](https://bowel.dev/en/docs/install/#first-steps).

Bazı özellikler platforma bağlıdır:

| | macOS | Windows | Linux | Android |
| --- | --- | --- | --- | --- |
| Devralma (Takeover) | Sistem vekil sunucusu veya TUN | Sistem vekil sunucusu veya TUN | Sistem vekil sunucusu (GNOME, KDE) veya TUN | Sistem VPN'i |
| Ağ geçidi modu | Evet | Hayır | Evet | Hayır |
| Geliştirici araçları | Evet | Hayır | Evet (zsh, bash) | Hayır |
| Ekrandaki QR kodlarını tarama | Evet | Evet | Evet | Hayır |
| Kamera ile QR kodlarını tarama | Evet | Hayır | Hayır | Evet |
| MCP sunucusu | Evet | Evet | Evet | Hayır |
| Komut satırı ve `bowel daemon` | Evet | Evet | Evet | Hayır |
| Uygulamaya göre yönlendirme (kurallar) | Evet | Evet, tarafımızca gerçek bir makinede hiç çalıştırılmadı | Evet | Android 10 veya üzeri |
| bowel'ı atlayan uygulamalar (hariç tutma listesi) | Yok | Yok | Yok | Evet, yeniden bağlandıktan sonra uygulanır |

Uygulamaya göre yönlendirme bir rota grubundaki bir kural türüdür (App), Android'deki hariç tutma listesi ise başka bir şeydir: listedeki bir uygulama VPN'i atlar ve hiçbir kural ona ulaşmaz. Her ikisi de [Rota grupları ve düğümler](https://bowel.dev/en/docs/routes/#routing-by-app) sayfasında açıklanmıştır.

TUN ilk açtığınızda bir kez yönetici izni ister. Her sistemde ne istediği ve Linux masaüstlerindeki sistem vekil sunucusu [Devralma, TUN ve çıkış kilidi](https://bowel.dev/en/docs/takeover/) sayfasında; her sistemin altyapıda neleri barındırdığı ve nelerin eksik olduğu [docs/platform-parity.md](docs/platform-parity.md) dosyasındadır.

## Sorular

En hızlı düğümün neden ChatGPT'yi açamadığı gibi daha fazlası [belgeler](https://bowel.dev/en/docs/faq/) sayfasındadır.

**Clash Verge Rev, Mihomo Party, FlClash, ClashX Meta veya Clash for Windows'tan mı geliyorsunuz?** Aynı abonelik bağlantısını Sağlayıcılar (Providers) altına ekleyin veya yapılandırma dosyasını içe aktarın. Bir abonelik veya dosya kendi rota grupları ve kuralları olan tam bir yapılandırma olduğunda, bowel bunlara göre mi yönlendireceğini yoksa yalnızca düğümlerini mi kullanacağını sorar. Sistem vekil sunucusunu veya TUN'u aynı anda yalnızca bir istemci devralmalıdır, bu yüzden önce eski istemciden çıkın.

**Hangi protokolleri destekliyor?** mihomo çekirdeğinin desteklediklerini: Shadowsocks, ShadowsocksR, VMess, VLESS (Reality ile), Trojan, Hysteria ve Hysteria2, TUIC, WireGuard, AnyTLS, Snell, Mieru, SOCKS5 ve HTTP; bir abonelikten, Clash yapılandırmasından veya `vless://`, `hy2://` ve `trojan://` gibi düğüm bağlantılarından.

**bowel herhangi bir veri topluyor mu?** Hayır. Telemetri, analitik ve hesap yoktur. Abonelik bağlantıları, düğümler ve ayarlar cihazınızdaki bowel veri klasöründe kalır ve bunlar ya da trafiğiniz hakkında projeye hiçbir şey gönderilmez. Güncellemeler bu deponun sürümlerine karşı kontrol edilir.

**bowel açık kaynak mı?** Kaynak kodu herkese açıktır; okuyabilir ve derleyebilirsiniz. Ancak lisansı olan [PolyForm Noncommercial License 1.0.0](LICENSE) bir OSI açık kaynak lisansı değildir: bowel kişisel ve diğer ticari olmayan kullanımlar için ücretsizdir; bir şirket için çalışma gibi ticari kullanımlar için [ticari lisans](#lisans) gerekir.

## Komut satırı

Aynı program, çalışan bowel için bir komut satırıdır. macOS'ta Homebrew ile kurulum `bowel` komutunu `PATH` içine ekler; aksi halde yolu `/Applications/bowel.app/Contents/MacOS/bowel` şeklindedir: `PATH` içine bağlayın (`ln -s /Applications/bowel.app/Contents/MacOS/bowel /usr/local/bin/bowel`) veya tam yolu kullanın. Komut olmadan çalıştırıldığında uygulamayı eskisi gibi açar.

```bash
bowel status                      # the light, mode, takeover, each route's node, traffic
bowel on                          # take over (system proxy); bowel off puts it back
bowel mode global                 # rule | global | direct
bowel routes                      # routes and the node each uses
bowel route use ai auto           # choose a route's node, or auto
bowel nodes --test                # nodes with fresh delays
bowel providers add <url|file>    # also: list, update, remove, enable, disable
bowel rules add openai.com ai     # also: list, remove, test <address>
bowel conns --watch               # connections as they open and end
bowel logs -f --level warning     # follow the core's log
bowel settings set logLevel debug # also: list, get
bowel tun on                      # and: bowel gateway on
bowel tui                         # all of it on a full screen
bowel bar                         # a line a second for a status bar
```

Her komutta örnekler içeren `--help` bulunur ve okuma komutları `--json` alır. Tam ekran `bowel tui`, waybar ve diğer durum çubukları için `bowel bar`, rota grubu başına vekil sunucu portu, çıkış kodları, penceresiz çalışma için `bowel daemon` ve ajanların neler yapabileceği [docs/cli.md](docs/cli.md) dosyasındadır.

## Geliştirme

Go 1.27.3+ gerektirir (`GOTOOLCHAIN=auto` otomatik indirir). Node veya Bun gerekmez.

```bash
./scripts/fetch-core.sh     # mihomo + GeoSite/GeoIP data into resources/
go tool mygo dev            # the app, rebuilt on every change
go tool mygo build          # bowel.app and a .dmg, signed but not notarized
./scripts/release.sh        # build, notarize and staple: use this for releases
CGO_ENABLED=0 go test ./... # includes tests that start the real core
./scripts/site-shots.sh     # re-render the README's and the website's screenshots (needs cwebp)
```

Herhangi bir masaüstü sisteminden Windows veya Linux için derlemek üzere önce o platformun çekirdeğini indirin. Windows kurulum dosyası ayrıca NSIS gerektirir.

```bash
GOOS=linux GOARCH=amd64 ./scripts/fetch-core.sh
go tool mygo build -platform linux/amd64 .
```

Mobil uygulamalar `mobile/` içindedir ve Flutter ile gomobile gerektirir; Android ayrıca Android SDK ve NDK gerektirir:

```bash
./mobile/android/build.sh       # Android: mihomo, the launcher and the bridge
./mobile/bridge/build.sh        # iOS: the bridge and the tunnel
go generate ./...               # the app's translations and skins, from the desktop's
cd mobile/flutter && flutter run
```

Flutter uygulamasının çeviri tablosu ve temaları masaüstünün Go kodundan üretilir ve güncel olmadıklarında `go test` başarısız olur. Masaüstünde olmayan kendi dizgeleri `internal/i18n/flutter.go` içindedir.

- Nasıl çalıştığı ve kodun nerede olduğu: [docs/architecture.md](docs/architecture.md)
- Ürünün bilgi mimarisi: [docs/product-ia.md](docs/product-ia.md)
- Web sitesi statiktir ve `site/` içindedir; önizlemek için `site/index.html` dosyasını açın. Belgeleri `docs/guide/` içindeki Markdown dosyalarından (ve `docs/cli*.md`, `docs/gateway.md`) oluşturulur: `pip install -r scripts/site-requirements.txt`, ardından `python3 scripts/build-site.py` tüm siteyi `build/site/` içine yazar. Değişiklikler `main` dalına ulaştığında yayına girer. bowel'ın nasıl kullanıldığına dair bir değişiklik, aynı pull request içinde her iki dildeki `docs/guide/` sayfasını da günceller.
- Sürüm yayınlamak için `mygo.json` içinde sürümü ayarlayın ve `v0.2.0` gibi eşleşen bir etiket (tag) gönderin. GitHub Actions macOS, Windows, Linux ve Android sürümlerini derler ve tek bir sürüm olarak yayınlar. iOS yalnızca iş akışı elle çalıştırıldığında derlenir ve yayınlanmaz.

bowel eskiden Moho olarak adlandırılıyordu. Go modülü hâlâ `github.com/oboo/moho` ve paket kimliği `dev.moho.app` şeklindedir; bu da macOS'in kamera ve ekran kaydı izinlerini korur. İlk açılışta `~/Library/Application Support/Moho` içindeki veriler `…/bowel` konumuna taşınır.

## Teşekkürler

- [mihomo](https://github.com/MetaCubeX/mihomo): vekil sunucu çekirdeği, GPL-3.0. Yayınlanan uygulamalar onu ayrı bir program olarak çalıştırır ve bağlamaz (linklemez). iOS bağlamak zorundadır, bu yüzden iOS uygulaması yayınlanmamaktadır.
- [MyGo](https://github.com/egoist/mygo): yerel Go arayüz araç takımı.
- [Flutter](https://flutter.dev): mobil uygulamaların arayüzü.
- [OPPO Sans](resources/fonts/): masaüstü uygulamalarının yazı tipi. Mobil uygulamalar sistem yazı tipini kullanır.

## Katkıda bulunanlar

<a href="https://github.com/21stware/bowel/graphs/contributors"><img src="https://contrib.rocks/image?repo=21stware/bowel" alt="Katkıda bulunanlar"></a>

## Lisans

bowel, yalnızca ticari olmayan kullanıma izin veren [PolyForm Noncommercial License 1.0.0](LICENSE) altında lisanslanmıştır. Ticari lisans için [bir issue açın](https://github.com/21stware/bowel/issues).

Linux dağıtımları, paket depoları ve uygulama mağazaları, bir işletme olup olmadıklarına bakılmaksızın [paketleme istisnası](PACKAGING-EXCEPTION.md) kapsamında bowel'ı kaynaktan derleyip dağıtabilir. Kullanıcıları yine bowel'ı yalnızca ticari olmayan amaçlarla edinir.

Pakete dahil edilen mihomo çekirdeği GPL-3.0 altında kalır. Masaüstü uygulamaları lisansını kaynakları arasında taşır (macOS'ta `Contents/Resources/licenses/mihomo/`), Android uygulaması Ayarlar'da adını belirtir ve her sürüm eşleşen kaynak kodu ekler. `assets/flags/` içindeki bayrak görselleri ve `resources/fonts/` içindeki yazı tipleri üçüncü taraf dosyalardır ve kendi lisanslarını korurlar.

## Yıldız geçmişi

<a href="https://star-history.com/#21stware/bowel&Date">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=21stware/bowel&type=Date&theme=dark">
    <img src="https://api.star-history.com/svg?repos=21stware/bowel&type=Date" width="600" alt="Yıldız geçmişi">
  </picture>
</a>
