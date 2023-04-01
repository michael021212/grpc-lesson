# grpc-lesson

以下の4つのメソッドを実装している。client/main.goで実行したいメソッドのコメントアウトを外し、
サーバー側ターミナルとクライアント側ターミナルでそれぞれ以下を実行する
```
$ go run server/main.go
$ go run client/main.go
```
- callListFiles(client) // Unary RPC のケース。ファイル名のリストを受け取る
- callDownload(client) // サーバーストリーミングのケース。分割ダウンロード
- callUpload(client) // クライアントストリーミングのケース。分割アップロード
- callUploadAndNotifyProgress(client) // 双方向ストリーミングのケース。分割アップロードと進捗のレスポンス

(注意)パスを伏せ字にしている
