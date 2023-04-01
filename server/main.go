package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"grpc-lesson/pb"
	"log"
	"net"
	"os"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware" // 'go get github.com/grpc-ecosystem/go-grpc-middleware'してから手動追加
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"  // 'go get github.com/grpc-ecosystem/go-grpc-middleware'してから手動追加
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// file_grpc.pb.goに定義されているUnimplementedFileServiceServer構造体を指定する
// server の実体はUnimplementedFileServiceServer ということ
type server struct {
	pb.UnimplementedFileServiceServer
}

// serverをレシーバとする?ListFiles()メソッドを定義
// storage/配下のファイル名を返す
func (*server) ListFiles(ctx context.Context, req *pb.ListFilesRequest) (*pb.ListFilesResponse, error) {
	fmt.Println("ListFiles was invoked")

	dir := "/Users/*****************/Desktop/udemy-grpc/grpc-lesson/storage"

	paths, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var filenames []string
	for _, path := range paths {
		if !path.IsDir() {
			filenames = append(filenames, path.Name())
		}
	}
	// filenames => [name.txt sports.txt]

	// file.pb.goのListFilesResponse構造体のFilenamesフィールドにファイル名を入れたインスタンスを受け取る
	res := &pb.ListFilesResponse{
		Filenames: filenames,
	}
	return res, nil
	// res => "filenames:"name.txt"  filenames:"sports.txt""
	// client側ではres.GetFilenames()を使って[name.txt sports.txt]という出力にする
}

// pb.DownloadRequest構造体のインスタンスと、pb.FileService_DownloadServerストリームを引数とするメソッド
func (*server) Download(req *pb.DownloadRequest, stream pb.FileService_DownloadServer) error {
	fmt.Println("Download was invoked")

	// .protoファイルにより生成されたGetFilenameメソッドを使う
	filename := req.GetFilename()
	path := "/Users/*****************/Desktop/udemy-grpc/grpc-lesson/storage/" + filename

	// ファイルの存在チェック
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return status.Error(codes.NotFound, "file was not found")
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// 長さが5のバイト型スライスを用意
	buf := make([]byte, 5)
	for {
		// Read は、データを与えられたバイトスライスへ入れ、入れたバイトのサイズとエラーの値を返します。
		// ストリームの終端は、 io.EOF のエラーで返します。
		// nには毎回5が入り、最後のループだけ3とかになる可能性がある
		n, err := file.Read(buf)
		if n == 0 || err == io.EOF {
			break
		}
		// 送信するデータがもうない場合はio.EOFがerrに入っており、return err が実行される
		if err != nil {
			return err
		}

		// bufにデータが読み込まれた場合、ストリームを介してクライアントにデータを転送する
		// Dataフィールドにbuf[:n]を入れて新しいpb.DownloadResponseオブジェクトを作成している
		// bufはインデックスが0~4までの5個の要素なので、buf[:n]のnが5の場合、n[:0]~n[:4]までの全てを指定したことになる
		res := &pb.DownloadResponse{Data: buf[:n]}
		sendErr := stream.Send(res)
		if sendErr != nil {
			return sendErr
		}
		// 今回は一瞬で処理が終わらないようにする
		time.Sleep(1 * time.Second)
	}

	// return nil をすると、ストリームはエンドオブファイルを返してレスポンスが終了する
	// Goではnilを返すことで、エラーが発生しなかったことを明示的に示す慣習がある
	return nil
}

func (*server) Upload(stream pb.FileService_UploadServer) error {
	fmt.Println("Upload was invoked")

	// クライアントからアップロードされたデータを格納するためのバッファーを用意
	var buf bytes.Buffer
	for {
		req, err := stream.Recv()
		// クライアントからの送信が終了した場合、サイズを返す
		if err == io.EOF {
			res := &pb.UploadResponse{Size: int32(buf.Len())}
			return stream.SendAndClose(res)
		}
		if err != nil {
			return err
		}

		data := req.GetData()
		log.Printf("recieved data(bytes): %v", data)
		log.Printf("recieved data(string): %v", string(data))
		buf.Write(data)
	}
}

func (*server) UploadAndNotifyProgress(stream pb.FileService_UploadAndNotifyProgressServer) error {
	fmt.Println("UploadAndNotifyProgress was invoked")

	size := 0

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		data := req.GetData()
		log.Printf("recieved data: %v", data)
		size += len(data)

		res := &pb.UploadAndNotifyProgressResponse{
			Msg: fmt.Sprintf("recieved %vbytes", size),
		}
		err = stream.Send(res)
		if err != nil {
			return err
		}
	}
}

// gRPCが提供する決まった形があり、それを満たす関数myLoggingというインターセプターを作る
// これをgrpc.NewServer()にオプションとして渡せばこのインターセプターを追加できる
func myLogging() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		log.Printf("request data: %+v", req)

		// handlerはクライアントからコールされるメソッド。この前後にlogを書くことで
		// "ListFiles was invoked"という出力の前後にログ出力ができるようになる
		resp, err = handler(ctx, req)
		if err != nil {
			return nil, err
		}
		log.Printf("response data: %+v", resp)

		return resp, nil
	}
}

// 認証のインターセプターを実装する
func authorize(ctx context.Context) (context.Context, error) {
	// クライアントから送られてきたメタデータからBearerというキーで認証情報を取り出す
	token, err := grpc_auth.AuthFromMD(ctx, "Bearer")
	if err != nil {
		return nil, err
	}

	if token != "test-token" {
		return nil, status.Error(codes.Unauthenticated, "token is invalid") // grpcのエラーコードを使う
	}
	return ctx, nil
}

func main() {
	lis, err := net.Listen("tcp", "localhost:50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// SSL通信のため、自前の証明書と秘密鍵を設定
	creds, err := credentials.NewServerTLSFromFile(
		"ssl/localhost.pem",
		"ssl/localhost-key.pem")
	if err != nil {
		log.Fatalln(err)
	}

	// grpcライブラリで定義されているNewServer関数を変数sに代入
	// NewServer()は、grpcライブラリで定義されているServer構造体のポインタを返す
	s := grpc.NewServer(
		grpc.Creds(creds),
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				myLogging(),
				grpc_auth.UnaryServerInterceptor(authorize)))) // 引数は応用編で追記。複数のインターセプターを書くにはChainUnaryServerを使う
	// file_grpc.pb.goに定義されているRegisterFileServiceServer関数を使う
	// 第一引数にgrpcサーバー、
	// 第二引数に今回はFileServiceServerインターフェースを実装した構造体:server(上部で定義)を渡す
	// grpcサーバーに構造体の内容を登録する
	// つまり、grpcサーバーが、ListFilesなどのメソッドを提供できるようになる
	pb.RegisterFileServiceServer(s, &server{})

	fmt.Println("server is runnning...")
	// .Serve(lis)で、lis(localhost:50051)でサーバーを起動
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
