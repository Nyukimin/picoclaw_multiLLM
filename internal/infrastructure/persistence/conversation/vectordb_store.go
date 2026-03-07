package conversation

import (
	"context"
	"fmt"
	"net"
	"strconv"
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
					Data: summary.Embedding,
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

	// Upsert（Wait=trueで同期書き込み）
	waitTrue := true
	_, err := v.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: v.collectionName,
		Points:         []*qdrant.PointStruct{point},
		Wait:           &waitTrue,
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

	limit := uint64(topK)
	// ベクトル検索（WithPayloadで要約情報も取得）
	searchResult, err := v.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: v.collectionName,
		Query: &qdrant.Query{
			Variant: &qdrant.Query_Nearest{
				Nearest: &qdrant.VectorInput{
					Variant: &qdrant.VectorInput_Dense{
						Dense: &qdrant.DenseVector{
							Data: queryEmbedding,
						},
					},
				},
			},
		},
		Limit:       &limit,
		WithPayload: &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
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
		summary.Score = point.Score
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// SearchByDomain はドメインでThread要約を検索
func (v *VectorDBStore) SearchByDomain(ctx context.Context, domain string, limit int) ([]*conversation.ThreadSummary, error) {
	lim := uint32(limit)
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
		Limit:       &lim,
		WithPayload: &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		WithVectors: &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search by domain: %w", err)
	}

	// 結果をThreadSummaryに変換
	summaries := make([]*conversation.ThreadSummary, 0, len(scrollResult))
	for _, point := range scrollResult {
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

	// SearchSimilar の実スコアを使用（Qdrant Cosine距離: 高いほど類似）
	similarity := topSummaries[0].Score
	isNovel := similarity < threshold

	return isNovel, similarity, nil
}

// --- ヘルパー関数 ---

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

// --- Knowledge Base (KB) メソッド ---

// getKBCollectionName はドメインごとのKBコレクション名を返す
func (v *VectorDBStore) getKBCollectionName(domain string) string {
	return fmt.Sprintf("kb_%s", domain)
}

// initKBCollection はKBコレクションを初期化
func (v *VectorDBStore) initKBCollection(ctx context.Context, domain string) error {
	collectionName := v.getKBCollectionName(domain)

	// コレクション存在確認
	exists, err := v.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("failed to check kb collection existence: %w", err)
	}

	if exists {
		return nil
	}

	// コレクション作成（embedding次元数: 768）
	err = v.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     768,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to create kb collection: %w", err)
	}

	// Payloadインデックス作成（source、created_at）
	_, err = v.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: collectionName,
		FieldName:      "source",
		FieldType:      qdrant.FieldType_FieldTypeKeyword.Enum(),
	})
	if err != nil {
		return fmt.Errorf("failed to create source index: %w", err)
	}

	return nil
}

// SaveKB はKnowledge BaseにDocumentを保存
func (v *VectorDBStore) SaveKB(ctx context.Context, doc *conversation.Document) error {
	if !doc.IsValid() {
		return fmt.Errorf("invalid document: missing required fields")
	}

	// KBコレクション初期化
	if err := v.initKBCollection(ctx, doc.Domain); err != nil {
		return err
	}

	collectionName := v.getKBCollectionName(doc.Domain)

	// Qdrant Point作成
	point := &qdrant.PointStruct{
		Id: &qdrant.PointId{
			PointIdOptions: &qdrant.PointId_Uuid{Uuid: doc.ID},
		},
		Vectors: &qdrant.Vectors{
			VectorsOptions: &qdrant.Vectors_Vector{
				Vector: &qdrant.Vector{
					Data: doc.Embedding,
				},
			},
		},
		Payload: map[string]*qdrant.Value{
			"content": {
				Kind: &qdrant.Value_StringValue{StringValue: doc.Content},
			},
			"source": {
				Kind: &qdrant.Value_StringValue{StringValue: doc.Source},
			},
			"created_at": {
				Kind: &qdrant.Value_IntegerValue{IntegerValue: doc.CreatedAt.Unix()},
			},
			"updated_at": {
				Kind: &qdrant.Value_IntegerValue{IntegerValue: doc.UpdatedAt.Unix()},
			},
		},
	}

	// Meta情報をPayloadに追加
	for key, value := range doc.Meta {
		if strVal, ok := value.(string); ok {
			point.Payload[key] = &qdrant.Value{
				Kind: &qdrant.Value_StringValue{StringValue: strVal},
			}
		}
	}

	// Upsert（Wait=trueで同期書き込み）
	waitTrue := true
	_, err := v.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collectionName,
		Points:         []*qdrant.PointStruct{point},
		Wait:           &waitTrue,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert kb document: %w", err)
	}

	return nil
}

// SearchKB はKnowledge BaseからDocumentを検索
func (v *VectorDBStore) SearchKB(ctx context.Context, domain string, queryEmbedding []float32, topK int) ([]*conversation.Document, error) {
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("queryEmbedding is empty")
	}

	collectionName := v.getKBCollectionName(domain)

	// コレクション存在確認
	exists, err := v.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to check kb collection existence: %w", err)
	}
	if !exists {
		// コレクションが存在しない場合は空の結果を返す
		return []*conversation.Document{}, nil
	}

	limit := uint64(topK)
	// ベクトル検索
	searchResult, err := v.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query: &qdrant.Query{
			Variant: &qdrant.Query_Nearest{
				Nearest: &qdrant.VectorInput{
					Variant: &qdrant.VectorInput_Dense{
						Dense: &qdrant.DenseVector{
							Data: queryEmbedding,
						},
					},
				},
			},
		},
		Limit:       &limit,
		WithPayload: &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search kb: %w", err)
	}

	// 結果をDocumentに変換
	docs := make([]*conversation.Document, 0, len(searchResult))
	for _, point := range searchResult {
		doc, err := pointToDocument(point, domain)
		if err != nil {
			// ログ出力してスキップ
			continue
		}
		doc.Score = point.Score
		docs = append(docs, doc)
	}

	return docs, nil
}

// pointToDocument はQdrant ScoredPointをDocumentに変換
func pointToDocument(point *qdrant.ScoredPoint, domain string) (*conversation.Document, error) {
	payload := point.Payload
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}

	// UUIDを取得
	var docID string
	if uuidID, ok := point.Id.GetPointIdOptions().(*qdrant.PointId_Uuid); ok {
		docID = uuidID.Uuid
	} else {
		return nil, fmt.Errorf("point id is not uuid")
	}

	doc := &conversation.Document{
		ID:     docID,
		Domain: domain,
		Meta:   make(map[string]interface{}),
	}

	// content
	if v, ok := payload["content"]; ok {
		if strVal, ok := v.GetKind().(*qdrant.Value_StringValue); ok {
			doc.Content = strVal.StringValue
		}
	}

	// source
	if v, ok := payload["source"]; ok {
		if strVal, ok := v.GetKind().(*qdrant.Value_StringValue); ok {
			doc.Source = strVal.StringValue
		}
	}

	// created_at
	if v, ok := payload["created_at"]; ok {
		if intVal, ok := v.GetKind().(*qdrant.Value_IntegerValue); ok {
			doc.CreatedAt = time.Unix(intVal.IntegerValue, 0)
		}
	}

	// updated_at
	if v, ok := payload["updated_at"]; ok {
		if intVal, ok := v.GetKind().(*qdrant.Value_IntegerValue); ok {
			doc.UpdatedAt = time.Unix(intVal.IntegerValue, 0)
		}
	}

	// その他のメタ情報
	for key, value := range payload {
		if key == "content" || key == "source" || key == "created_at" || key == "updated_at" {
			continue
		}
		if strVal, ok := value.GetKind().(*qdrant.Value_StringValue); ok {
			doc.Meta[key] = strVal.StringValue
		}
	}

	return doc, nil
}

// ListKBDocuments はKBコレクション内の全ドキュメントを取得（ページング対応）
func (v *VectorDBStore) ListKBDocuments(ctx context.Context, domain string, limit int) ([]*conversation.Document, error) {
	collectionName := v.getKBCollectionName(domain)

	// コレクション存在確認
	exists, err := v.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to check kb collection existence: %w", err)
	}
	if !exists {
		return []*conversation.Document{}, nil
	}

	lim := uint32(limit)

	// Scroll でドキュメント取得（フィルタなし）
	scrollResult, err := v.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: collectionName,
		Limit:          &lim,
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		WithVectors:    &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list kb documents: %w", err)
	}

	// 結果をDocumentに変換
	docs := make([]*conversation.Document, 0, len(scrollResult))
	for _, point := range scrollResult {
		doc, err := retrievedPointToDocument(point, domain)
		if err != nil {
			continue
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// retrievedPointToDocument はQdrant RetrievedPointをDocumentに変換
func retrievedPointToDocument(point *qdrant.RetrievedPoint, domain string) (*conversation.Document, error) {
	payload := point.Payload
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}

	// UUIDを取得
	var docID string
	if uuidID, ok := point.Id.GetPointIdOptions().(*qdrant.PointId_Uuid); ok {
		docID = uuidID.Uuid
	} else {
		return nil, fmt.Errorf("point id is not uuid")
	}

	doc := &conversation.Document{
		ID:     docID,
		Domain: domain,
		Meta:   make(map[string]interface{}),
	}

	// content
	if v, ok := payload["content"]; ok {
		if strVal, ok := v.GetKind().(*qdrant.Value_StringValue); ok {
			doc.Content = strVal.StringValue
		}
	}

	// source
	if v, ok := payload["source"]; ok {
		if strVal, ok := v.GetKind().(*qdrant.Value_StringValue); ok {
			doc.Source = strVal.StringValue
		}
	}

	// created_at
	if v, ok := payload["created_at"]; ok {
		if intVal, ok := v.GetKind().(*qdrant.Value_IntegerValue); ok {
			doc.CreatedAt = time.Unix(intVal.IntegerValue, 0)
		}
	}

	// updated_at
	if v, ok := payload["updated_at"]; ok {
		if intVal, ok := v.GetKind().(*qdrant.Value_IntegerValue); ok {
			doc.UpdatedAt = time.Unix(intVal.IntegerValue, 0)
		}
	}

	// その他のメタ情報
	for key, value := range payload {
		if key == "content" || key == "source" || key == "created_at" || key == "updated_at" {
			continue
		}
		if strVal, ok := value.GetKind().(*qdrant.Value_StringValue); ok {
			doc.Meta[key] = strVal.StringValue
		}
	}

	return doc, nil
}

// GetKBCollections は存在するKBコレクション一覧を取得
// NOTE: Qdrant Go client のListCollections APIが不安定なため、
// 既知のドメインリストから存在確認する簡易実装
func (v *VectorDBStore) GetKBCollections(ctx context.Context) ([]string, error) {
	// 既知のドメインリスト（実運用では設定から読み込むべき）
	knownDomains := []string{"general", "programming", "movie", "anime", "tech", "history"}

	existingDomains := make([]string, 0)
	for _, domain := range knownDomains {
		collectionName := v.getKBCollectionName(domain)
		exists, err := v.client.CollectionExists(ctx, collectionName)
		if err != nil {
			continue
		}
		if exists {
			existingDomains = append(existingDomains, domain)
		}
	}

	return existingDomains, nil
}

// GetKBStats はKBコレクションの統計情報を取得
func (v *VectorDBStore) GetKBStats(ctx context.Context, domain string) (*KBStats, error) {
	collectionName := v.getKBCollectionName(domain)

	// コレクション存在確認
	exists, err := v.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to check collection existence: %w", err)
	}
	if !exists {
		return &KBStats{Domain: domain, DocumentCount: 0, VectorSize: 768}, nil
	}

	// ドキュメント数をカウント（Scroll で取得して数える簡易実装）
	limit := uint32(1000)
	scrollResult, err := v.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: collectionName,
		Limit:          &limit,
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: false}},
		WithVectors:    &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}

	return &KBStats{
		Domain:        domain,
		DocumentCount: len(scrollResult),
		VectorSize:    768, // 固定値（設定から読み込むべき）
	}, nil
}

// KBStats はKBコレクションの統計情報
type KBStats struct {
	Domain        string
	DocumentCount int
	VectorSize    int
}

// DeleteOldKBDocuments は指定日時より古いKBドキュメントを削除
func (v *VectorDBStore) DeleteOldKBDocuments(ctx context.Context, domain string, before time.Time) (int, error) {
	collectionName := v.getKBCollectionName(domain)

	// コレクション存在確認
	exists, err := v.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return 0, fmt.Errorf("failed to check collection existence: %w", err)
	}
	if !exists {
		return 0, nil
	}

	// created_at < before のドキュメントを削除
	beforeUnix := before.Unix()
	beforeFloat := float64(beforeUnix)
	_, err = v.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collectionName,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
				Filter: &qdrant.Filter{
					Must: []*qdrant.Condition{
						{
							ConditionOneOf: &qdrant.Condition_Field{
								Field: &qdrant.FieldCondition{
									Key: "created_at",
									Range: &qdrant.Range{
										Lt: &beforeFloat,
									},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("failed to delete old documents: %w", err)
	}

	// 削除数は正確には取得できないため、簡易実装として0を返す
	// 実運用では削除前後でカウントして差分を返すべき
	return 0, nil
}
