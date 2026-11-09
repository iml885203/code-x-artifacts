# internal/auth

本機帳號以私人 JSON 的 bcrypt hash 驗證；選用 LDAP 時以登入者本人對 AD bind，讀 profile 和遞迴群組；不碰 session。

- 本機帳號在啟動時驗證姓名、唯一帳號、bcrypt cost 與容量；錯誤設定拒絕啟動，錯誤密碼不回 profile。
  Tests: `TestLocalAccountsAuthenticateAndExposeOnlyTheConfiguredRoster`, `TestLocalAccountConfigurationFailsClosed`.

## 不變式

- **登入時從同一次 bind、同一個查詢讀本人的 `department`，沒有就存空字串（使用量顯示 `unknown`）；不從群組推斷部門。**
  Tests: `TestASignInStoresTheDepartmentADReportsAndUnknownWhenItReportsNone` (internal/web).

- **密碼不會以明文經過網路：只有 `ldaps://` 才用 simple bind，其他一律 NTLM；空密碼不送到 AD（AD 會把它當成匿名 bind 放行）。**
  Tests: `TestThePasswordGoesInClearOnlyInsideTLS`, `TestEmptyPasswordNeverReachesTheDirectory`.

- **服務不持有任何 AD 憑證：以登入者本人 bind，同一條連線讀自己的資料和群組。部署不需要 secret；session key 第一次啟動時產生並存在 DB 裡。**

- **同一條已綁定連線的組織名錄讀取只提供查找資料，不改登入政策。只有群組範圍、全部分頁和每筆資料驗證完成才產生完整快照；失敗不拒絕已通過的登入、不保存憑證或連線。**
  Tests: `TestDirectoryPagingIsCompleteOnlyAfterAValidatedTerminalPage`, `TestDirectoryGroupScopeMustResolveInsideTheConfiguredBase`, `TestDirectoryFailureAndCancellationCannotBecomeAnEmptyCompleteRoster`.
