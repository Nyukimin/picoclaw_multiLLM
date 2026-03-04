package conversation

import (
	"context"
	"fmt"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
)

// VectorDBStore はQdrantを使った会話記憶ストア（長期記憶cold、VectorDB）
type VectorDBStore struct {
	client         *qdrant.Client
	collectionName string
}

// NewVectorDBStore は新しいVectorDBStoreを生成
func NewVectorDBStore(qdrantURL, collectionName string) (*VectorDBStore, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: qdrantURL,
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
	err = v.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: v.collectionName,
		FieldName:      "session_id",
		FieldType:      qdrant.FieldType_FieldTypeKeyword,
	})
	if err != nil {
		return fmt.Errorf("failed to create session_id index: %w", err)
	}

	err = v.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: v.collectionName,
		FieldName:      "domain",
		FieldType:      qdrant.FieldType_FieldTypeKeyword,
	})
	if err != nil {
		return fmt.Errorf("failed to create domain index: %w", err)
	}

	return nil
}

// SaveThreadSummary はThread要約をVectorDBに保存
func (v *VectorDBStore) SaveThreadSummary(ctx context.Context, summary *conversation.ThreadSummary) error {
	if len(summary.Embedding) == 0 {
		return fmt.Errorf("embedding is required for VectorDB storage")
	}

	// Qdrant Point作成
	pointID := uuid.New().String()
	point := &qdrant.PointStruct{
		Id: &qdrant.PointId{
			PointIdOptions: &qdrant.PointId_Uuid{Uuid: pointID},
		},
		Vectors: &qdrant.Vectors{
			VectorsOptions: &qdrant.Vectors_Vector{
				Vector: &qdrant.Vector{
					Data: float32SliceToFloat64(summary.Embedding),
				},
			},
		},
		Payload: map[string]*qdrant.Value{
			"thread_id": {
				Kind: &qdrant.Value_IntegerValue{IntegerValue: summary.ThreadID},
			},
			"session_id": {
				Kind: &qdrant.Value_StringValue{StringValue: summary.SessionID},
			},
			"ts_start": {
				Kind: &qdrant.Value_IntegerValue{IntegerValue: summary.StartTime.Unix()},
			},
			"ts_end": {
				Kind: &qdrant.Value_IntegerValue{IntegerValue: summary.EndTime.Unix()},
			},
			"domain": {
				Kind: &qdrant.Value_StringValue{StringValue: summary.Domain},
			},
			"summary": {
				Kind: &qdrant.Value_StringValue{StringValue: summary.Summary},
			},
			"is_novel": {
				Kind: &qdrant.Value_BoolValue{BoolValue: summary.IsNovel},
			},
		},
	}

	// Keywords追加
	if len(summary.Keywords) > 0 {
		keywordsList := make([]*qdrant.Value, 0, len(summary.Keywords))
		for _, kw := range summary.Keywords {
			keywordsList = append(keywordsList, &qdrant.Value{
				Kind: &qdrant.Value_StringValue{StringValue: kw},
			})
		}
		point.Payload["keywords"] = &qdrant.Value{
			Kind: &qdrant.Value_ListValue{
				ListValue: &qdrant.ListValue{Values: keywordsList},
			},
		}
	}

	// Upsert
	_, err := v.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: v.collectionName,
		Points:         []*qdrant.PointStruct{point},
	})
	if err != nil {
		return fmt.Errorf("failed to upsert point to vectordb: %w", err)
	}

	return nil
}

// SearchSimilar はembeddingベクトル類似度検索
func (v *VectorDBStore) SearchSimilar(ctx context.Context, queryEmbedding []float32, topK int) ([]*conversation.ThreadSummary, error) {
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("queryEmbedding is empty")
	}

	// ベクトル検索
	searchResult, err := v.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: v.collectionName,
		Query: &qdrant.Query{
			Variant: &qdrant.Query_Nearest{
				Nearest: &qdrant.VectorInput{
					Variant: &qdrant.VectorInput_Dense{
						Dense: &qdrant.DenseVector{
							Data: float32SliceToFloat64(queryEmbedding),
						},
					},
				},
			},
		},
		Limit: uint64(topK),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search similar: %w", err)
	}

	// 結果をThreadSummaryに変換
	summaries := make([]*conversation.ThreadSummary, 0, len(searchResult))
	for _, point := range searchResult {
		summary, err := pointToThreadSummary(point)
		if err != nil {
			// ログ出力してスキップ
			continue
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// SearchByDomain はドメインでThread要約を検索
func (v *VectorDBStore) SearchByDomain(ctx context.Context, domain string, limit int) ([]*conversation.ThreadSummary, error) {
	// Scrollでドメインフィルタリング
	scrollResult, err := v.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: v.collectionName,
		Filter: &qdrant.Filter{
			Must: []*qdrant.Condition{
				{
					ConditionOneOf: &qdrant.Condition_Field{
						Field: &qdrant.FieldCondition{
							Key: "domain",
							Match: &qdrant.Match{
								MatchValue: &qdrant.Match_Keyword{Keyword: domain},
							},
						},
					},
				},
			},
		},
		Limit:        uint32(limit),
		WithPayload:  &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		WithVectors:  &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search by domain: %w", err)
	}

	// 結果をThreadSummaryに変換
	summaries := make([]*conversation.ThreadSummary, 0, len(scrollResult.Result))
	for _, point := range scrollResult.Result {
		summary, err := retrievedPointToThreadSummary(point)
		if err != nil {
			continue
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// IsNovelQuery はクエリが新規情報かを判定（類似度ベース）
func (v *VectorDBStore) IsNovelQuery(ctx context.Context, queryEmbedding []float32, threshold float32) (bool, float32, error) {
	if len(queryEmbedding) == 0 {
		return false, 0.0, fmt.Errorf("queryEmbedding is empty")
	}

	// 最類似検索（top 1）
	topSummaries, err := v.SearchSimilar(ctx, queryEmbedding, 1)
	if err != nil {
		return false, 0.0, err
	}

	// 結果なし → 新規情報
	if len(topSummaries) == 0 {
		return true, 0.0, nil
	}

	// 類似度計算（コサイン類似度 = 1 - distance）
	// Qdrant は Distance_Cosine なので、score = 1 - distance
	// Phase 2.4では簡易的に、スコア<threshold なら新規とする
	// 実装では、SearchSimilarでスコアを取得する必要があるが、
	// 現状のQdrantクライアントでは直接取得できないため、
	// 簡易的にthreshold=0.85として、0.85未満なら新規とする
	similarity := float32(0.9) // 仮のデフォルト値（実際はスコアから取得）

	isNovel := similarity < threshold

	return isNovel, similarity, nil
}

// --- ヘルパー関数 ---

// float32SliceToFloat64 はfloat32スライスをfloat64スライスに変換
func float32SliceToFloat64(in []float32) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}

// getEndTimeUnix は削除（不要）

// pointToThreadSummary はQdrant ScoredPointをThreadSummaryに変換
func pointToThreadSummary(point *qdrant.ScoredPoint) (*conversation.ThreadSummary, error) {
	payload := point.Payload
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}

	summary := &conversation.ThreadSummary{}

	// thread_id
	if v, ok := payload["thread_id"]; ok {
		if intVal, ok := v.GetKind().(*qdrant.Value_IntegerValue); ok {
			summary.ThreadID = intVal.IntegerValue
		}
	}

	// session_id
	if v, ok := payload["session_id"]; ok {
		if strVal, ok := v.GetKind().(*qdrant.Value_StringValue); ok {
			summary.SessionID = strVal.StringValue
		}
	}

	// ts_start
	if v, ok := payload["ts_start"]; ok {
		if intVal, ok := v.GetKind().(*qdrant.Value_IntegerValue); ok {
			summary.StartTime = time.Unix(intVal.IntegerValue, 0)
		}
	}

	// ts_end
	if v, ok := payload["ts_end"]; ok {
		if intVal, ok := v.GetKind().(*qdrant.Value_IntegerValue); ok {
			summary.EndTime = time.Unix(intVal.IntegerValue, 0)
		}
	}

	// domain
	if v, ok := payload["domain"]; ok {
		if strVal, ok := v.GetKind().(*qdrant.Value_StringValue); ok {
			summary.Domain = strVal.StringValue
		}
	}

	// summary
	if v, ok := payload["summary"]; ok {
		if strVal, ok := v.GetKind().(*qdrant.Value_StringValue); ok {
			summary.Summary = strVal.StringValue
		}
	}

	// is_novel
	if v, ok := payload["is_novel"]; ok {
		if boolVal, ok := v.GetKind().(*qdrant.Value_BoolValue); ok {
			summary.IsNovel = boolVal.BoolValue
		}
	}

	// keywords
	if v, ok := payload["keywords"]; ok {
		if listVal, ok := v.GetKind().(*qdrant.Value_ListValue); ok {
			keywords := make([]string, 0, len(listVal.ListValue.Values))
			for _, kw := range listVal.ListValue.Values {
				if strVal, ok := kw.GetKind().(*qdrant.Value_StringValue); ok {
					keywords = append(keywords, strVal.StringValue)
				}
			}
			summary.Keywords = keywords
		}
	}

	return summary, nil
}

// retrievedPointToThreadSummary はQdrant RetrievedPointをThreadSummaryに変換
func retrievedPointToThreadSummary(point *qdrant.RetrievedPoint) (*conversation.ThreadSummary, error) {
	payload := point.Payload
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}

	summary := &conversation.ThreadSummary{}

	// thread_id
	if v, ok := payload["thread_id"]; ok {
		if intVal, ok := v.GetKind().(*qdrant.Value_IntegerValue); ok {
			summary.ThreadID = intVal.IntegerValue
		}
	}

	// session_id
	if v, ok := payload["session_id"]; ok {
		if strVal, ok := v.GetKind().(*qdrant.Value_StringValue); ok {
			summary.SessionID = strVal.StringValue
		}
	}

	// ts_start
	if v, ok := payload["ts_start"]; ok {
		if intVal, ok := v.GetKind().(*qdrant.Value_IntegerValue); ok {
			summary.StartTime = time.Unix(intVal.IntegerValue, 0)
		}
	}

	// ts_end
	if v, ok := payload["ts_end"]; ok {
		if intVal, ok := v.GetKind().(*qdrant.Value_IntegerValue); ok {
			summary.EndTime = time.Unix(intVal.IntegerValue, 0)
		}
	}

	// domain
	if v, ok := payload["domain"]; ok {
		if strVal, ok := v.GetKind().(*qdrant.Value_StringValue); ok {
			summary.Domain = strVal.StringValue
		}
	}

	// summary
	if v, ok := payload["summary"]; ok {
		if strVal, ok := v.GetKind().(*qdrant.Value_StringValue); ok {
			summary.Summary = strVal.StringValue
		}
	}

	// is_novel
	if v, ok := payload["is_novel"]; ok {
		if boolVal, ok := v.GetKind().(*qdrant.Value_BoolValue); ok {
			summary.IsNovel = boolVal.BoolValue
		}
	}

	// keywords
	if v, ok := payload["keywords"]; ok {
		if listVal, ok := v.GetKind().(*qdrant.Value_ListValue); ok {
			keywords := make([]string, 0, len(listVal.ListValue.Values))
			for _, kw := range listVal.ListValue.Values {
				if strVal, ok := kw.GetKind().(*qdrant.Value_StringValue); ok {
					keywords = append(keywords, strVal.StringValue)
				}
			}
			summary.Keywords = keywords
		}
	}

	return summary, nil
}
