# web（React 前端）

React + TypeScript + Vite 前端；自己的 go.mod 只為了讓 `./...` 不走進 node_modules。npm、版本釘死、`package-lock.json` 提交；Node 版本在 `.node-version`。
bundle 在 image build 時建好、`embed` 進 binary（`internal/web/bundle.go`），執行時沒有 Node。`src/App.tsx` 依 boot 選頁；`src/editor/`（Docs 編輯器，有自己的 AGENTS.md，獨立 chunk）；`src/app.css` 是元件樣式，只用 `internal/web/static/style.css` 的 token；`src/i18n.tsx` 取用 server 給的文字。

## 不變式

- **RoomPresence uses the provider's existing connection states: Live has a steady green light, connecting/reconnecting a dashed ring, offline a hollow square, each with translated text and an accessible status label. The Docs host hides the normal live banner while keeping RoomPresence mounted to publish collaborators; connecting, reconnecting and offline remain visible. Other hosts retain their own presentation. No continuous animation.** Tests: web/src/presence/RoomPresence.test.tsx (announces each actual connection state with visible text in both languages), web/src/tokens.test.ts (connected light contrast on every surface in both themes).

- **介面提示共用 `Tooltip`（Radix），hover 延遲 350ms、鍵盤聚焦可看、Escape 可關；內容只放說明，不放互動控制；trigger 保留自己的無障礙名稱，不疊加原生 title，portal 以視窗邊界避讓。**
  Tests: web/src/Tooltip.test.tsx（describes a focused trigger through a portal and dismisses on Escape without changing its name、delays a pointer hint and closes when the pointer leaves、lets a hinted dropdown trigger open its menu without a competing hint）。

- **前端只經由既有的 JSON API（cookie + `X-Pictor`）取資料，不自己判斷誰能看什麼（用 `canChange`、`canEdit`、`canComment` 決定顯示）；登入頁和 tokens 頁用原生 form post，不經 JS；CSP 不因 React 放寬，只多了 `img-src data: blob:`（白板編輯器畫在畫布上的圖示，與它上傳前縮小圖片用的 blob；兩者都不能執行 script、也連不到別的網站）與 `style-src` 的一個 hash（白板匯出 SVG 時加在文字上的 `white-space: pre;`，只放行這個值）。** bridge 的外層（`src/viewer/bridge.ts`）只回應自己 iframe 的訊息，且只認 `operations` 裡有的操作。
  Tests（前端）：各頁的元件測試 gallery/Gallery.test.tsx、gallery/filter.test.ts、viewer/Viewer.test.tsx、viewer/bridge.test.ts、pages.test.tsx；CSP：`TestTheShellAsksNothingOfTheCSPThatTheOtherPagesDoNotAsk`。

- **frame 顯示舊版時 bridge 是唯讀的：`whoami`、`db.get`、`db.list` 照常，`db.set`、`db.delete` 一律拒絕，免得舊版的程式覆蓋目前版本在用的共享資料；是否唯讀由 viewer 依自己開 frame 時帶的版本決定，不看頁面說什麼，切換版本時 frame 重建。** 版本紀錄面板（作者與編輯者才有）：點一列在 frame 唯讀看那一版（`/raw/{id}?version=N`），Export 選單的下載跟著那一版（`export/{format}?version=N`），「還原」問過一次後以打開時看到的版本當 `baseVersion` 送出，被拒（409）就告訴作者有更新的版本、什麼都不變。
  Tests（前端）：viewer/Viewer.test.tsx 的 version history 一組（含 downloads the version being viewed…）、viewer/bridge.test.ts 的 refuses writes from a frame showing an older version；版本面板把同一批人（作者與 agent 都相同）、距該組最新版 10 分鐘內的連續版本收成一組，只顯示組內最新版，其餘一鍵展開、每一版都點得到（還原自別版的版本自成一組），列上列出作者與 agent（「某某的 Claude Code」），預覽舊版時工具列沒有「編輯／完成」。規則在 viewer/versionGroups.ts：viewer/versionGroups.test.ts、Viewer.test.tsx 的 grouping／names the agent／offers no way to edit 各條；e2e/tests/versions.spec.ts。

- **頁面要交一個檔案給看的人，只能呼叫 `pictor.download({name, type, data})`，檔案不經頁面自己存：viewer 先以自己的規則檢查（`viewer/download/rules.ts`：最多 10 MB、型別只有 text／csv／json／markdown／pdf／png／jpeg／gif／webp／zip、檔名去掉路徑、控制字元與方向記號、不留開頭的點、最長 100 字、副檔名與型別相符），不合的在問人之前就拒絕；合的才跳出確認（檔名、型別、大小），本人按「儲存」才存、存下的就是頁面送來的位元組；取消、關閉或離開都是拒絕，promise 以明確的錯誤 reject。** 每次呼叫各問一次（接受一次不代表下一次），本人還沒回答時再來的呼叫直接拒絕，頁面在本人決定時被新版本換掉也拒絕；完整頁面（「在新分頁開啟」，`/a/{id}?view=full`）也用同一個 Viewer、bridge 與確認，只有 raw iframe 填滿 viewport，不開 Docs editor 或 Design canvas；可帶 `version=N` 固定唯讀版本。沒有 viewer 的地方（`/raw/{id}` 單獨開啟）不能存。frame 的 sandbox 沒有 `allow-downloads`。html 與 react 頁面走同一個 bridge。限制寫在 `kinds` 的 html 與 react guide，由測試對照。
  Tests（前端）：viewer/download/rules.test.ts（檔名與型別與大小邊界）、viewer/download/Download.test.tsx（確認內容、接受存下一模一樣的位元組、拒絕、超限或不在清單不問人、各自要確認、不堆疊、html 與 react 相同）、viewer/bridge.test.ts（頁面被換掉時拒絕、沒有人可問時拒絕）、viewer/bridgeFrame.test.ts（頁面端：Blob 讀成位元組、單獨開啟時拒絕）、viewer/download/guides.test.ts（guide 的限制與規則相同）；Go：`TestTheHTMLAndReactGuidesTeachCodeXArtifactsDownloadWithItsLimitsAndThatTheViewerMayRefuse`；e2e/tests/bridge.spec.ts（接受後瀏覽器收到那個檔名、第二次被取消就沒有存）。

- **開著的 viewer 每 10 秒（`pollEveryMs`）問一次 `changes`：** 別人發佈了新版本就換一個新 frame 並把捲動位置帶過去（位置只是數字，只認自己 frame 送的）；新留言或可見範圍／編輯者的變動則重讀 artifact、不重載 frame。分頁隱藏時不問、顯示時立刻問；正在看舊版時不問；artifact 被刪或登出（404／401）就停。
  Tests（前端）：viewer/Follow.test.tsx；e2e/tests/follow.spec.ts。

- **作者和編輯者可以在 viewer 管理 artifact 的檔案（「⋯」選單的「檔案」，`canEdit` 才有）：** 面板只經由 `PUT/GET/DELETE /api/artifacts/{id}/assets`，server 拒絕時把 server 的原因（`ApiError.reason`）原文顯示在那個檔名下，不自己判斷能不能上傳。
  Tests: web/src/viewer/Viewer.test.tsx 的 files 一組（gives the author a Files item and a reader none、shows the server's reason when it refuses a file）、e2e/tests/files.spec.ts。

- **viewer 的頂端列對每一種 artifact 都相同（`viewer/TopBar.tsx`）：返回、種類、標題選單、可見範圍、作者與版本，右邊是 presence、留言、分享、overflow 與帳號；手機收起次要操作。權限只在單一 Share dialog 管理：作者可改 audience、依登入帳號增刪編輯者、開關組織編輯；其他人只看目前權限與複製連結。每次變更立即儲存，一次一個，等待時保留已存狀態並停用權限控制；成功後用 server 回應更新角色，失敗保留已存狀態與可重試的錯誤。組織編輯開關在 organisation 與 public 範圍顯示，private 隱藏；private 保留指定編輯者介面，切回 organisation 不自動恢復已清除的組織編輯。私人範圍不啟用組織編輯；公開匿名者永遠顯示唯讀，組織成員另列。Done、Escape 與從外面按下並放開的 backdrop 點擊只關閉，不還原已存變更；內距與從內拖外不關閉。原生 modal 困住 focus，關閉還給 Share。公開連結停用時說明原因，已選的 public 維持可用。**
  Tests（前端）：viewer/Viewer.test.tsx（bar、audience、Undo、copy、public links、reader）、viewer/ShareAccess.test.tsx（add/remove、組織編輯、saved roles/reload、失敗、public/private、pending dismissal）、viewer/ShareDismiss.test.tsx（backdrop/interior/drag/Escape）；e2e/tests/viewer.spec.ts（由 Share 授權同事並改名）。

- **「Connect your agent」（`/connect`）頁的 token 只存在於頁面的記憶體：`POST /api/tokens` 的回應是它唯一出現的地方，不在 boot 區塊、不在網址、不寫 log、不進瀏覽器儲存，重新載入就沒了；** 它只給瀏覽器的 session，帶 token 的請求不能建立 token；各 client 的指令用這台 server 自己的網址（boot 的 `connect.baseUrl`），建立前用 `<YOUR_TOKEN>`。
  Tests: `TestATokenMadeFromTheConnectPageComesBackOnceAndWorksOnMCPButOnlyABrowserSessionCanMakeOne`, `TestTheConnectPageHandsTheAppThisServersAddressAndTheTextOfTheSetupInBothLanguages`；web/src/connect/Connect.test.tsx（turns the name field into the token, fills it into every tab and keeps it only in memory、is gone when the page is opened again、the snippets）；瀏覽器：e2e/tests/connect.spec.ts。

- **時間一律以相對說法顯示，精確時間留給 hover 與無障礙：卡片、viewer 中繼資料、版本紀錄、留言、檔案、Connect 的 token 清單都用 `RelativeTime`（`<time datetime tabindex aria-label>`，精確時間由共用 `Tooltip` 在 hover／focus 顯示並以 `aria-describedby` 關聯），不自己格式化時間；比現在晚的時間當作「現在」；整頁只有一個計時器（`time.ts` 的 `useNow`），分頁隱藏時不跑。** 規則在 `time.ts`（`relativeSpan`、`relativeTime`），語言由 `I18nProvider` 的 `lang` 給。
  Tests: web/src/time.test.ts（各單位、en 與 zh-Hant、未來時間與時鐘差）、web/src/RelativeTime.test.tsx、各處的時間：gallery/Gallery.test.tsx（shows relative times…）、viewer/Viewer.test.tsx（版本、留言、檔案各一）、connect/Connect.test.tsx；e2e/tests/gallery-cards.spec.ts（a card's time moves from now to a minute ago）。

- **人看得到每個 artifact 的種類（網頁、文件、簡報、設計），但只用圖示與顏色，名稱只在 tooltip 與 `aria-label`、畫面上不寫字；一律經 `kinds/` 的 `<KindMark kind size>`：`kinds/kinds.ts` 的一張表是唯一知道種類長相的地方（html 與 react 同為網頁、不認得的種類也是網頁；儀表板、動畫、設計系統只有顏色與圖示、標為預留），兩個主題的底色與圖示色來自 `DESIGN.md`、圖示色在底色上 ≥ 4.5:1。畫面上沒有「複製給 agent」，任何地方都不顯示格式版本。**
  Tests: web/src/kinds/kinds.test.tsx、web/src/artifactActionList.test.ts（選單的項目）、web/src/viewer/Viewer.test.tsx（names a … artifact’s kind only in the mark’s tooltip…、offers no copy for an agent）、web/src/gallery/Gallery.test.tsx（the card menu 一組）。

- **作品庫（`/`）左邊是 220px 的導覽（作品與數量、最近開啟（`/recent`）、給 agent 的說明、Connect、Kit，底部是帳號按鈕）。** 上方一列是搜尋（`/` 聚焦）、分組選單、格狀／列表切換與「New」；範圍分頁（全部、我的、分享給我的、已釘選）附數量，目前的有星色底線。
  Tests: web/src/gallery/Gallery.test.tsx（opens the rail…、starts something new from the top row…、counts what each filter holds…）。

- **總覽的卡片只用圖示表達可見範圍（有名稱與 tooltip、沒有文字）與種類（卡片底部左側的 `<KindMark>`）；每張卡片是一個有邊框的方塊：標題最多兩行並以 tooltip 給全名，中繼資料列（種類標、標題、相對時間與作者、範圍圖示、未讀數）固定在卡片底部；釘子與「⋯」在預覽右上角成一組、同一列的卡片等高；每個只有圖示的按鈕都有名稱與 tooltip；預覽 iframe 是 `/raw/{id}?preview`、`sandbox="allow-scripts"`、`loading="lazy"`，所以 script 畫的頁面也看得到、不在 console 留錯誤。卡片的「⋯」（hover、focus 或選單開啟時出現；無 hover 裝置上保持可見）與列表列的「⋯」是同一份（`gallery/itemActions.tsx`）並與 viewer 的「⋯」共用 `artifactActions.ts`：釘選 P、重新命名 R（在卡片上原地改，給能編輯的人，與 viewer 的標題同一條規則）、建立副本、複製連結 C、刪除（只給 `canChange` 的人且問過一次）；選單開著時按字母就執行；沒有「複製給 agent」。**
  Tests: web/src/gallery/Gallery.test.tsx（gives every card an audience icon…、clamps a long title…、gives every icon-only control a name and a tooltip、the card menu 一組）、web/src/artifactActionList.test.ts、e2e/tests/gallery-cards.spec.ts。

- **總覽可以分組（不分組、編輯日期、擁有者、種類）並切換格狀或列表；兩個選擇各人各自記在這個瀏覽器（`pictor.library.group.<帳號>`、`pictor.library.view.<帳號>`），儲存空間不能用或丟錯就是「不分組、格狀」。** 每個作品剛好在一個群組裡，釘選的排在每個群組最前面，沒有作品的群組不畫；擁有者先列自己、其他人依名字；依種類時群組標題用種類的顏色。列表的每一列是 `--row-h` 高的無邊框列：種類方塊、單行標題（tooltip 給全名）、未讀留言的數字圓點，右邊是釘子、範圍圖示、相對時間與「⋯」。規則在 `gallery/group.ts`。
  Tests: web/src/gallery/group.test.ts（每種分組、日期邊界、儲存空間丟錯）、web/src/gallery/Gallery.test.tsx 的 grouping and the list 一組；e2e/tests/gallery-cards.spec.ts（carol does not see bob's pin, groups the library by owner as a list…）。

- **釘選是個人的：卡片的釘子、卡片「⋮」與 viewer「⋯」都經 `PUT`／`DELETE /api/artifacts/{id}/pin`，畫面先換、伺服器拒絕就換回並提示；總覽把釘選的排最前面，「已釘選」篩選只列釘選的；沒登入的人沒有釘選項。**
  Tests: web/src/gallery/filter.test.ts、web/src/gallery/Gallery.test.tsx 的 pinning 一組、web/src/viewer/Viewer.test.tsx 的 pinning 一組；e2e/tests/gallery-cards.spec.ts（carol finds a card by search, pins it with P and renames it…、carol does not see bob's pin…）。

- **「New」（總覽的上方一列）與 viewer 標題的改名只經 `POST /api/new` 與 `PUT /api/artifacts/{id}/title`，改名按鈕只給 `canEdit` 的人；新文件開在編輯器（網址 `?edit`），新設計開在畫布。**
  Tests: web/src/create/Create.test.tsx；e2e/tests/create.spec.ts。

- **viewer 標題列的右上角顯示現在正在看的人，每一種 artifact 都是同一個（手機收進「⋯」）：頁面開著時每 20 秒 `POST /api/artifacts/{id}/viewers`（隱藏時不送、顯示時立刻送，404 或 401 就停），一個共用的 `AvatarStack` 顯示前 4 個頭像、agent 有 AI 圖示、訪客標「訪客」；hover 或 focus 頭像顯示完整名字（agent 是「帳號 的 client」加 agent 標記），點任一頭像或「+N」打開列出所有人的小視窗（每列有頭像、名字、agent、訪客或「你」標記），Esc、點外面或 focus 離開就關閉；同一個人不論來自 heartbeat 或 live room 都只出現一次，訪客永遠不和人或 agent 合併。沒登入的頁面以 `presence/guest.ts` 的瀏覽器隨機 id（`localStorage`，被擋時 `sessionStorage`，再被擋時只在頁面記憶體；不做指紋）當訪客送 heartbeat，只看到自己；登入的頁面只送它以前當訪客的 id，讓帳號取代它；訪客名單滿了（429）就不顯示、不記錯誤，照 `Retry-After` 再試，403 當作已登出而停止。** 前端不判斷誰能看到名單，名單就是 server 的回答（規則在 `internal/service/AGENTS.md`）。
  Docs 編輯器與 Design 畫布不另畫名單：它們的 `RoomPresence` 把 room 的 awareness 交給同一個堆疊，自己只畫連線燈。
  Tests: web/src/presence/people.test.ts、web/src/presence/awarenessPeople.test.ts、web/src/presence/AvatarStack.test.tsx、web/src/presence/Viewing.test.tsx（含 room 的人與訪客）、web/src/presence/guest.test.ts、web/src/design/PeersOnCanvas.test.tsx（頭像堆疊與連線燈）、web/src/viewer/Viewer.test.tsx 的 who is viewing、guests of a … artifact 與 signed-out visitor 各條；e2e/tests/follow.spec.ts（alice and bob see each other…）、e2e/tests/viewer.spec.ts（a public link opens without signing in, its visitors are guests…）、e2e/tests/design-presence.spec.ts（…and guests join the stack）。
- **選單（總覽的「New」與帳號選單、卡片的「⋮」、viewer 的「⋯」）關閉時不把 focus 搶回按鈕：Radix 在選單離開頁面後的下一個 task 才把 focus 還給按鈕，選單淡出的那一瞬間點進文件的人，打的字會進按鈕；所以每個選單的 `onCloseAutoFocus` 都用 `menuFocus.ts` 的 `keepFocusWhereItWent`，focus 已在別處就留在那裡，沒有人動才還給按鈕。**
  Tests: web/src/create/Create.test.tsx（leaves focus where the person put it…）。

- **語言與主題只在頭像選單（`shell/AccountMenu.tsx`）：目前的有勾，改了立刻生效、不重新載入；主題記在這個瀏覽器（儲存被擋時仍可切換，只是不記住）、語言由 server 記住（同一個 `POST /lang`，回應的 boot 區塊帶新的目錄）；登入頁沒有頭像，語言選單在登入外框右上方（手機版與標誌同列），共用 Radix radio menu 與 menu tokens，選項以原生 `POST /lang` 表單提交並保留 `next`，Escape 還焦點、點進欄位不搶回焦點。** 登入失敗的訊息在欄位下面（`role="alert"`，兩個欄位 `aria-invalid`），帳號不清掉。
  登入頁以品牌幾何與產品介紹搭配帳號表單；不啟動背景 canvas。表單錯誤、語言切換、焦點與兩個主題持續可用。
  Tests: web/src/login/Login.test.tsx、web/src/pages.test.tsx、web/src/tokens.test.ts；e2e/tests/signin.spec.ts。

- **Design 的 PDF 與 Doc 共用 viewer 的列印路徑：** Export 選單只讀 server 的格式宣告；畫布沒有可列印的 `/raw` 頁面，所以畫面外載入目前看的版本（包含所有 page），載入後才交給既有 bridge 等字型、圖片與 frame 後列印，不印 viewer、不呼叫 server PDF 路由。列印 frame 的樣式放在 viewer，切換版本便移除它，不能列印先前留下的版本。
  Tests: web/src/viewer/Viewer.test.tsx（prints the design being viewed, including an older version…）、viewer/Editing.test.tsx（prints a doc open in the editor…）；e2e/tests/design-export.spec.ts（reader 從 PDF 選單列印全設計，frame 的字型與圖片載入，三個不同大小的頁面保留文字）。


- **簡報在 viewer 是舞台加底部縮圖列（`slides/`）：舞台是 deck 自己的頁面（`/raw/{id}?slide=N&embed`，server 的 sandbox 與 bridge，播放器的控制列只在簡報模式出現），shell 只用訊息請它換頁（`{pictor:"slides", show}`）、只聽自己 frame 說的頁碼；縮圖是 deck 的預覽（`/raw/{id}?preview#slide-N`，沒有 script、`sandbox="allow-scripts"`、不接收指標），所以 deck 的任何內容都不在 sandbox 外執行。編輯者的新增（在選中的之後）、建立副本、拖曳或 Alt+方向鍵移動、刪除（Delete 問過一次）都經 #42 的 slide 路由並帶縮圖列顯示的版本當 `baseVersion`，顯示回應的清單；409 就讀回較新的清單與版本、說明變更沒有存、什麼都不改。讀者（包括沒登入、開公開簡報的人）有同一組縮圖、單張／格狀與縮放，沒有任何編輯的控制；清單讀不到時在縮圖列說明並可重試。一次只送一個變更，另一個在存檔中被要求時告訴本人稍等；刪除帶的是問本人時縮圖列的版本，所以中間換了簡報就是 409、不會刪到別張；頁面晚到的、已被新選擇取代的回報不會把選擇拉回去，回報的頁碼限制在簡報之內。**
  Tests: web/src/slides/SlidesStage.test.tsx（每一條）、web/src/slides/order.test.ts；e2e/tests/slides.spec.ts。

- **編輯者在舞台上就地改投影片上的字、從左上的插入工具加元素（`slides/SlideFields.tsx`、`SlideTools.tsx`）：欄位是 shell 自己蓋在 frame 上的 textarea，位置來自舞台 frame 自己送的 `{pictor:"fields"}`（只認它的 `contentWindow`、只認目前這張、數字有界、路徑必須是依元素種類 server 會標的位置、且在 slide JSON 裡是字串，`placement.ts`、`fields.ts` 的 `editable`），字一律從 `GET …/slides/{n}` 的 JSON 取（`fields.ts`，每次改都回新的投影片），不從頁面讀。Enter 或點開別處存、Escape 不存、Shift+Enter 只在文字元素換行、輸入法組字中的 Enter 留給輸入法。每次存都是 `replace_slide`（`PUT …/slides/{n}`）送整張投影片，以讀到它的版本當 `baseVersion`；409 就顯示較新的簡報（讀不到也照樣保留）、保留打的字並提供「套用到較新的投影片」：只在較新的投影片那個位置仍是改之前的字時才套用，否則只保留文字給人複製；還不能存（投影片正在重讀）時欄位留著、請人稍候。插入在投影片最後加一個有效預設內容的文字、清單、表格、卡片或大數字（滿 8 個就停用），加好後若人還在那一張，打開它的第一個欄位。復原／重做（按鈕與 Ctrl/Cmd+Z、Shift+Z、Ctrl+Y）把這次工作階段的改動放回去，可以一步步往回與往前，只在簡報仍是這次工作階段最後存出的版本時可用（別人、agent 或縮圖列的改動都會結束它）。清單說可能超出畫面的投影片，舞台上說一聲。讀者、格狀檢視與舊版沒有欄位也沒有插入工具。**
  Tests: web/src/slides/SlideEditing.test.tsx（每一條）、web/src/slides/fields.test.ts；e2e/tests/slides.spec.ts。

- **作品庫帳號按鈕內的頭像固定為不收縮的 32px 正方形圓形；外層按鈕保留觸控目標。**
  Tests: web/src/shell/AccountMenu.test.tsx（keeps the rail avatar a non-shrinking square inside the account target）。

- **OAuth 啟用時，Connect 優先顯示無靜態 token 的 MCP 網址與瀏覽器登入說明，PAT 設定留在可展開區塊；OAuth 授權清單只取 server 回傳的本人授權，載入失敗可重試，撤銷前確認、失敗保留該列。Consent 是原生 POST form，保留 server 驗證過的隱藏欄位與 consent token，明確顯示未驗證的 client 名稱、client ID、返回網址、既有帳號權限與持續連接行為；允許與拒絕由 server 決定。** Tests: web/src/connect/OAuth.test.tsx（OAuth-first/PAT fallback、list retry、revoke cancel/failure、bilingual consent fields and permissions）。

## 坑

- 元件測試用真的目錄文字、假的 `fetch`，測使用者看到與做的事，不測實作細節；純邏輯另寫 table test。
- 前端的測試（vitest）列在上面各條的 Tests 之後，`verify-agents-md` 只檢查 Go 測試名稱，前端的測試檔名要自己跟著改。
- 不碰 DOM 的測試檔第一行寫 `// @vitest-environment node`：jsdom 每個檔案要重新載入，是這類檔案大部分的啟動時間。需要 DOM 的（包括 ProseMirror 的 view）漏寫只會變慢，寫錯會直接失敗。

- Own-comment deletion uses the server capability and a confirmation dialog; cancellation/failure preserves content. A deleted root labels its tombstone and keeps surviving replies. Earlier reads cannot resurrect deleted content. Tests: web/src/viewer/CommentsPanel.test.tsx, web/src/viewer/useComments.test.ts.

- **白板（`board`）在 viewer 裡是 Excalidraw（`src/board/`，自己的 chunk，只有白板的頁面載入），接在白板的 room 上：每個元素的每個欄位一個 entry，只寫這個人的編輯器改過的元素；別人的改動以 `CaptureUpdateAction.NEVER` 進場景，所以不進自己的 Undo；標籤的位置每次都從它的框重排（與 `boardfmt.LayoutLabels` 相同的算法）；Excalidraw 在交出 API 之後才非同步載入第一個場景（`initializeScene`），載入會換掉場景、清掉 Undo，所以要等它載入完（第一次 change 事件，`appState.isLoading` 是 false；`board/loaded.ts`）才綁上 room，否則先放上去的白板會被清空；編輯器在接收 room 狀態時自己做的正規化（空的標籤被標成刪除）不寫回 room，否則會刪掉別人剛開始打的字；agent 寫的元素缺的欄位由 Excalidraw 的 `restoreElements` 補上預設值。讀者（`canEdit` false）是 view mode、provider 唯讀、不寫任何東西。字型隨 bundle 放在 `/assets/excalidraw/`（`EXCALIDRAW_ASSET_PATH` 由 entry 設定），Excalidraw 內建的 CDN 備援在 build 時改成站內位址（`vite.config.ts`，版本不同時 build 失敗），所以頁面不向別的網站要任何東西。嵌入網頁與 AI 工具不提供。白板與 Design 畫布的鍵相同（`src/board/keys.ts`，在 Excalidraw 之前攔下）：+／= 與 - 用 Design 的級距放大縮小、0 符合畫面、1 是 100%（Excalidraw 本來的 0 橡皮擦、1 選取在白板上不用，工具列上的這兩個提示也不顯示）；H、V、按住空白鍵拖曳本來就一致。介面語言跟著 CodeXArtifacts（zh-Hant 是 Excalidraw 的 zh-TW）。「New」選單的每一種都寫出它是用來做什麼的（白板：畫流程、架構和想法；設計：頁面的樣子），名稱是選項的 accessible name、用途是 description；空白白板由 `POST /api/new` 建立。白板的 Export：開著編輯器時 SVG 與 PNG 是 Excalidraw 自己畫的（`exportToSvg` 不內嵌字型，因為內嵌要跑 WebAssembly；PNG 用頁面已有的字型），檔名照 server 的規則取自標題（`board/exportBoard.ts`）；沒有編輯器（舊版本、完整頁面）時 SVG 從 server 下載、PNG 由瀏覽器從那張 SVG 畫成。沒有字型的文字用 Excalidraw 的 Excalifont。貼上或拖進白板的圖片經 `generateIdForFile` 上傳成 artifact 的檔案（`uploadName` 取名，伺服器拒絕時顯示它的原因、什麼都不加），檔名就是圖片的 `fileId`；每個瀏覽器只為 artifact 檔案清單裡有的檔案從 `/files/` 載入，已刪的不去要，Excalidraw 畫它的空框（`board/files.ts`）。**
  Tests: web/src/board/binding.test.ts（每一條）、web/src/board/loaded.test.ts、web/src/board/keys.test.ts、web/src/board/exportBoard.test.ts、web/src/board/files.test.ts、web/src/create/Create.test.tsx（says what each kind is for…、makes a board from Board…、is in Chinese…）、`TestANewBoardIsAnEmptyBoardPrivateToItsAuthorWithADefaultTitle`；e2e/tests/board.spec.ts（兩個人同時畫、注音標籤、綁定的箭頭、游標、重新載入、讀者、沒有站外請求；一人搬框一人改字、兩邊都留著且標籤置中、Undo 只收回自己的；1、+、-、0 改縮放，空白鍵與 H 平移、V 回到選取；匯出 SVG 與 PNG、圖庫卡片畫出白板、未登入的人看到畫好的白板而沒有工具）。

- **OIDC 登入沿用同一個品牌登入頁，但只顯示已設定 provider 的明確登入／重試連結（server 的 `login.oidc.startUrl`），不自動跳轉、不顯示本機密碼欄位；取消、過期／驗證失敗與 provider 無法連線都有雙語錯誤及可重新開始的動作。語言切換保留原本的登入目的地。** Tests: web/src/login/Login.test.tsx（OIDC sign-in）。
