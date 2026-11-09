# 頁面交給看的人下載的檔案

html 與 react 頁面用 `pictor.download({name, type, data})` 請看的人儲存檔案。一般 viewer 與「在新分頁開啟」的完整頁面（`/a/{id}?view=full`）使用同一個受信任的 Viewer、bridge、檔名／型別／大小規則和確認對話框；每個要求都要本人按「儲存」，取消會讓頁面收到拒絕，接受一次不會授權下一次。具體參數與限制見 MCP `kinds` 的 html／react guide。

完整頁面沿用 `/a/{id}` 的權限檢查與 boot，隱藏 viewer 頂端列，讓原來的 raw iframe 填滿 viewport；iframe 保留 server 提供的 sandbox，不加 `allow-downloads` 或 `allow-same-origin`。下載由外層在本人同意後儲存，頁面不取得 cookie 或 API 權限。直接開 `/raw/{id}` 仍是沒有受信任 parent 的隔離內容，不能要求下載。

一般 viewer 與完整頁面都跟隨目前版本的更新；完整頁面加 `version=N` 固定顯示該版，bridge 拒絕共享資料寫入，讀取與下載仍可用，版本是否可讀由既有的 server service 決定。完整頁面的 Docs 是已存文件的閱讀呈現，Design 是既有 raw 總覽；一般 viewer 的即時文件編輯器、Design 畫布與 Play 不受影響。Slides 的 `slide=N` 繼續指向指定投影片。
