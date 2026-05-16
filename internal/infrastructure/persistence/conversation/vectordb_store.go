package conversation

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/qdrant/go-client/qdrant"
)

// VectorDBStore はQdrantを使った会話記憶ストア（長期記憶cold、VectorDB）
type VectorDBStore struct {
	client         *qdrant.Client
	collectionName string
}

// NewVectorDBStore は新しいVectorDBStoreを生成
// qdrantURL は "host:port" 形式（例: "localhost:6333"）
func NewVectorDBStore(qdrantURL, collectionName string) (*VectorDBStore, error) {
	host, portStr, err := net.SplitHostPort(qdrantURL)
	if err != nil {
		// コロンがない場合はホスト名のみとして扱い、デフォルトgRPCポート(6334)を使用
		host = qdrantURL
		portStr = "6334"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid qdrant port %q: %w", portStr, err)
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host:                   host,
		Port:                   port,
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant client: %w", err)
	}

	store := &VectorDBStore{
		client:         client,
		collectionName: collectionName,
	}

	// コレクション初期化
	if err := store.initCollection(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize collection: %w", err)
	}

	return store, nil
}

// Close はQdrant接続を閉じる
func (v *VectorDBStore) Close() error {
	if v.client != nil {
		return v.client.Close()
	}
	return nil
}

// initCollection はコレクションを初期化
func (v *VectorDBStore) initCollection(ctx context.Context) error {
	// コレクション存在確認
	exists, err := v.client.CollectionExists(ctx, v.collectionName)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if exists {
		return nil
	}

	// コレクション作成（embedding次元数: 768、Cohere/OpenAI embedding想定）
	err = v.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: v.collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     768,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	// Payloadインデックス作成（session_id、domain、ts_start）
	_, err = v.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: v.collectionName,
		FieldName:      "session_id",
		FieldType:      qdrant.FieldType_FieldTypeKeyword.Enum(),
	})
	if err != nil {
		return fmt.Errorf("failed to create session_id index: %w", err)
	}

	_, err = v.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: v.collectionName,
		FieldName:      "domain",
		FieldType:      qdrant.FieldType_FieldTypeKeyword.Enum(),
	})
	if err != nil {
		return fmt.Errorf("failed to create domain index: %w", err)
	}

	return nil
}

// --- ヘルパー関数 ---

// --- Knowledge Base (KB) メソッド ---
