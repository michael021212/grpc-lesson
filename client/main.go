package main

import (
	"context"
	"fmt"
	"grpc-lesson/pb"
	"io"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func main() {
	// SSL通信のための実装
	certFile := "/Users/*****************/Library/Application Support/mkcert/rootCA.pem"
	creds, err := credentials.NewClientTLSFromFile(certFile, "")
	// サーバーとの接続を確立する
	// WithInsecureは通信が暗号化されないので本番では使わない
	// 代わりにWithTransportCredentialsを使う
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	// main関数の終了時に必ず接続をcloseする
	defer conn.Close()

	// fileServiceClient構造体を取得してclientに代入
	// file_grpc.pb.goにあるfileServiceClientのListFilesメソッドを呼び出せるようにするため
	client := pb.NewFileServiceClient(conn)

	// callListFiles(client) // Unary RPC のケース
	// callDownload(client) // サーバーストリーミングのケース
	// callUpload(client) // クライアントストリーミングのケース
	callUploadAndNotifyProgress(client) // 双方向ストリーミングのケース
}

// pb.FileServiceClient型のclientを引数に受け取り、
func callListFiles(client pb.FileServiceClient) {
	md := metadata.New(map[string]string{"authorization": "Bearer bad-token"})
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	// file_grpc.pb.goに定義されているListFilesメソッド通りに実装する
	// レシーバーはfileServiceClientのポインタとなっている
	// ListFilesメソッドの第一引数には空のContextを渡し、
	// 第二引数にfile.pb.goのListFilesRequest構造体を渡す
	// ListFilesResponseが返ってくる
	res, err := client.ListFiles(ctx, &pb.ListFilesRequest{})
	if err != nil {
		log.Fatalln(err)
	}

	// filenamesのゲッターメソッドがfile.pb.goに生成されているのでそれを使う
	fmt.Println(res.GetFilenames())
}

func callDownload(client pb.FileServiceClient) {
	// タイムアウトの実装
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &pb.DownloadRequest{Filename: "name.txt"}
	// FileService_DownloadClientインターフェースがstreamに入る
	stream, err := client.Download(ctx, req)
	if err != nil {
		log.Fatalln(err)
	}

	for {
		// stream(FileService_DownloadClientインターフェース)のRecv()メソッドの返り値は*DownloadResponse
		res, err := stream.Recv()
		// サーバーからEOFが到達するとループを抜ける
		if err == io.EOF {
			break
		}
		if err != nil {
			resErr, ok := status.FromError(err) // grpcのエラーの場合はokがtrueになる
			if ok {
				if resErr.Code() == codes.NotFound {
					log.Fatalf("Error Code: %v, Error Message: %v", resErr.Code(), resErr.Message())
				} else if resErr.Code() == codes.DeadlineExceeded {
					log.Fatalln("deadline exceeded")
				} else {
					log.Fatalln("unknown grpc error")
				}
			} else {
				log.Fatalln(err)
			}
		}

		log.Printf("Response from Donwload(bytes): %v", res.GetData())
		log.Printf("Response from Donwload(string): %v", string(res.GetData()))
	}
}

func callUpload(client pb.FileServiceClient) {
	filename := "sports.txt"
	path := "/Users/*****************/Desktop/udemy-grpc/grpc-lesson/storage/" + filename

	file, err := os.Open(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer file.Close()

	// // FileService_UploadClientインターフェースがstreamに入る
	stream, err := client.Upload(context.Background())
	if err != nil {
		log.Fatalln(err)
	}

	// アップロードするデータを入れる変数を作る
	buf := make([]byte, 5)
	for {
		// データをbufに入れる。nにはサイズが入る
		n, err := file.Read(buf)
		if n == 0 || err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalln(err)
		}

		// UploadRequestのオブジェクトを作って、streamに流す
		req := &pb.UploadRequest{Data: buf[:n]}
		sendErr := stream.Send(req)
		if sendErr != nil {
			log.Fatalln(sendErr)
		}

		time.Sleep(1 * time.Second)
	}

	// リクエストの終了をサーバーに通知し、レスポンスを受け取る
	res, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalln(err)
	}

	log.Printf("recieved data size: %v", res.GetSize())
}

func callUploadAndNotifyProgress(client pb.FileServiceClient) {
	filename := "sports.txt"
	path := "/Users/*****************/Desktop/udemy-grpc/grpc-lesson/storage/" + filename

	file, err := os.Open(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer file.Close()

	stream, err := client.UploadAndNotifyProgress(context.Background())
	if err != nil {
		log.Fatalln(err)
	}

	// request
	buf := make([]byte, 5)
	go func() {
		for {
			n, err := file.Read(buf)
			if n == 0 || err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalln(err)
			}

			req := &pb.UploadAndNotifyProgressRequest{Data: buf[:n]}
			sendErr := stream.Send(req)
			if sendErr != nil {
				log.Fatalln(sendErr)
			}
			time.Sleep(1 * time.Second)
		}

		err := stream.CloseSend()
		if err != nil {
			log.Fatalln(err)
		}
	}()

	// response
	ch := make(chan struct{})
	go func() {
		for {
			res, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalln(err)
			}

			log.Printf("recieved message: %v", res.GetMsg())
		}
		close(ch)
	}()
	<-ch
}
