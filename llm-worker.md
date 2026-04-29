# LLM Worker Overview

このドキュメントは、Synthify の LLM worker を「今の実装がどう動くか」という観点で読むための概要である。
ツール仕様の詳細は [../docs/llm-worker-tools.md](../docs/llm-worker-tools.md) を参照。

## 1. 役割

LLM worker は、アップロード済みドキュメントを読んで、知識ツリーに変換するバックグラウンド実行系である。

責務は大きく 4 つ:

- ソースファイルを取得してテキスト化する
- 文書を chunk に分割し、知識 item を組み立てる
- capability を確認しながら DB に item / source / mutation log を保存する
- 進捗を Firestore に流して frontend にリアルタイム通知する

API backend は job の作成と認可を担当し、worker は重い処理を担当する。

## 2. エントリポイント

worker server の起動点は [main.go](/home/unix/Synthify/worker/cmd/server/main.go)。

ここで行っていること:

- config を読む
- Store を初期化する
- Firestore notifier を初期化する
- Gemini API key があれば ADK model を作る
- `WorkerService` の Connect handler を公開する

公開 RPC は [worker.proto](/home/unix/Synthify/proto/synthify/tree/v1/worker.proto) の 3 つ:

- `GenerateExecutionPlan`
- `ExecuteApprovedPlan`
- `EvaluateJobArtifact`

Connect handler 実装は [connect.go](/home/unix/Synthify/worker/pkg/worker/connect.go)。

## 3. 処理フロー

通常フローは次の通り。

1. frontend が backend API に `startProcessing` を送る
2. backend が OLTP DB に processing job を作る
3. backend が worker に `GenerateExecutionPlan` を送る
4. backend が worker に `ExecuteApprovedPlan` を送る
5. worker が file を読んで chunk / item を生成する
6. worker が DB に保存する
7. worker が Firestore に progress を流す
8. frontend は Firestore `onSnapshot` で状態を受ける
9. 完了後、frontend は backend API から tree や logs を読み直す

## 4. Worker 内部

主処理は [worker.go](/home/unix/Synthify/worker/pkg/worker/worker.go) の `Process`。

高レベルにはこう動く。

1. request を検証する
2. job / document の存在を確認する
3. job status を `running` にする
4. `processDocument` を実行する
5. 成功なら job を `succeeded` にする
6. 失敗なら job を `failed` にする

`processDocument` の中では次の順で進む。

1. `text_extraction`
   - `file_uri` から source file を取得する
   - 取れなければ既存 `document_chunks` を再利用する
2. `semantic_chunking`
   - テキストを見出しベースで分割する
   - `document_chunks` に保存する
3. `goal_driven_synthesis`
   - chunk ごとに `pipeline.SynthesizedItem` を作る
4. `persistence`
   - `JobCapability` を確認する
   - `tree_items` と `item_sources` を保存する
   - `job_mutation_logs` を残す

## 5. LLM と fallback

worker は ADK agent を初期化するが、今の実装では deterministic pipeline が主経路である。

- LLM が使える場合:
  - ADK agent は起動される
  - ただし現在は best-effort review に近い使い方で、主保存経路は deterministic code
- LLM が使えない場合:
  - worker はそのまま動く
  - chunking / synthesis / persistence はコードベースの fallback で進む

このため、現在の worker は「LLM を使えるときに補助的に使う」構成で、完全な agent 主導ではない。

## 6. ツール群

orchestrator に登録されている tool 群は [orchestrator.go](/home/unix/Synthify/worker/pkg/worker/agents/orchestrator.go) にある。

代表例:

- `extract_text`
- `semantic_chunking`
- `goal_driven_synthesis`
- `persist_knowledge_tree`
- `generate_brief`
- `quality_critique`
- `semantic_search`

ただし、すべてが本格的な LLM tool として完成しているわけではない。
いくつかは deterministic 実装または薄い placeholder に近い。

## 7. 永続化の境界

worker が OLTP DB に書いてよいのは、capability を満たす mutation に限る。

境界は主に次の API で表現されている。

- `CreateStructuredItemWithCapability`
- `UpsertItemSource`
- `UpdateItemSummaryHTMLWithCapability`
- `UpsertJobExecutionPlan`
- `UpsertJobEvaluation`

つまり worker は「何でも書ける」のではなく、job capability を前提に書く。

## 8. Firestore status

job progress は [notifier.go](/home/unix/Synthify/shared/jobstatus/notifier.go) から Firestore に出る。

path:

```text
workspaces/{workspaceId}/jobs/{jobId}
```

主な field:

- `status`
- `currentStage`
- `progress`
- `message`
- `errorMessage`
- `createdAt`
- `startedAt`
- `updatedAt`
- `completedAt`

これは frontend 用の projection であり、正本ではない。
正本は PostgreSQL 側の job / tree / mutation log である。

## 9. Frontend からの見え方

frontend は Firestore を直接購読して進捗を表示する。

- 単一 job: `useJobStatus`
- workspace の最近の job: `useWorkspaceJobStatuses`

完了したら backend API へ再問い合わせして、tree や mutation logs を読む。
Firestore の内容だけで UI を確定しないのが前提。

## 10. 今の制約

現状の worker にはいくつか制約がある。

- LLM agent は主経路ではなく補助経路
- semantic search は OLTP 上の bounded fallback query が中心
- chunking / synthesis はまだ軽量実装
- Firestore rules は job status read を暫定的に広めに許可している
- workspace membership を Firestore rules で厳密に見るための mirror は未実装

## 11. どこを見ると理解しやすいか

最初に読む順番はこれがよい。

1. [worker.go](/home/unix/Synthify/worker/pkg/worker/worker.go)
2. [connect.go](/home/unix/Synthify/worker/pkg/worker/connect.go)
3. [notifier.go](/home/unix/Synthify/shared/jobstatus/notifier.go)
4. [orchestrator.go](/home/unix/Synthify/worker/pkg/worker/agents/orchestrator.go)
5. [llm-worker-tools.md](/home/unix/Synthify/docs/llm-worker-tools.md)
