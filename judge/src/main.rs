use axum::{extract::State, http::StatusCode, routing::{get, post}, Json, Router};
use serde::{Deserialize, Serialize};
use std::{process::Stdio, sync::Arc, time::{SystemTime, UNIX_EPOCH}};
use tokio::{io::AsyncWriteExt, process::Command, time::{timeout, Duration}};

#[derive(Clone)] struct AppState { runner_image: String }
#[derive(Deserialize)] struct JudgeRequest { #[serde(rename="submissionId")] submission_id: String, #[serde(rename="taskId")] task_id: String, language: String, code: String }
#[derive(Serialize)] struct JudgeResponse { status: String, message: String, passed: usize, total: usize }
#[derive(Clone)] struct Test { input: &'static str, output: &'static str }
async fn health() -> Json<serde_json::Value> { Json(serde_json::json!({"status":"ok","service":"avernaarena-judge","runner":"docker-isolated-python"})) }
async fn judge(State(state): State<Arc<AppState>>, Json(request): Json<JudgeRequest>) -> Result<Json<JudgeResponse>, (StatusCode, Json<serde_json::Value>)> {
    if request.code.is_empty() || request.code.len() > 65_536 { return Err((StatusCode::BAD_REQUEST, Json(serde_json::json!({"error":"code must be 1..65536 bytes"})))); }
    if request.language != "python" { return Ok(Json(JudgeResponse { status:"compile_error".into(), message:"Изолированный runner пока поддерживает Python 3.12.".into(), passed:0, total:0 })); }
    let tests = match request.task_id.as_str() {
        "sum-two" => vec![Test{input:"2 3\n",output:"5"},Test{input:"-4 9\n",output:"5"},Test{input:"0 0\n",output:"0"}],
        "max-three" => vec![Test{input:"4 9 2\n",output:"9"},Test{input:"-1 -7 -3\n",output:"-1"},Test{input:"8 8 2\n",output:"8"}],
        "reverse-string" => vec![Test{input:"arena\n",output:"anera"},Test{input:"a\n",output:"a"},Test{input:"\n",output:""}],
        "balanced-brackets" => vec![Test{input:"(()())\n",output:"YES"},Test{input:"(()\n",output:"NO"},Test{input:")(" ,output:"NO"}],
        "two-sum" => vec![Test{input:"4 9\n2 7 11 15\n",output:"0 1"},Test{input:"3 6\n3 2 4\n",output:"1 2"}],
        _ => return Err((StatusCode::NOT_FOUND, Json(serde_json::json!({"error":"unknown task"}))))
    };
    let nonce = SystemTime::now().duration_since(UNIX_EPOCH).unwrap_or_default().as_nanos();
    let mut passed = 0usize;
    let mut final_status = "accepted";
    let mut message = "Все тесты пройдены".to_string();
    for (test_index, test) in tests.iter().enumerate() {
        let container_name = format!("aa-{}-{}-{}", std::process::id(), nonce, test_index);
        let mut child = Command::new("docker")
            .args(["run", "--rm", "--pull=missing", "--network=none", "--memory=128m", "--memory-swap=128m", "--cpus=0.5", "--pids-limit=32", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--user=65534:65534", "--log-driver=none", "--tmpfs", "/tmp:rw,noexec,nosuid,size=16m", "--name"])
            .arg(&container_name)
            .args(["-w", "/tmp"])
            .arg(&state.runner_image)
            .args(["python", "-I", "-c"])
            .arg(&request.code)
            .stdin(Stdio::piped()).stdout(Stdio::piped()).stderr(Stdio::piped()).kill_on_drop(true).spawn().map_err(internal)?;
        if let Some(mut stdin) = child.stdin.take() { stdin.write_all(test.input.as_bytes()).await.map_err(internal)?; }
        let output = match timeout(Duration::from_secs(5), child.wait_with_output()).await {
            Ok(Ok(output)) => output,
            Ok(Err(_)) => { final_status="runtime_error"; message="Не удалось запустить изолированный процесс".into(); break; },
            Err(_) => { let _ = Command::new("docker").arg("kill").arg(&container_name).output().await; let _ = Command::new("docker").args(["rm", "-f"]).arg(&container_name).output().await; final_status="time_limit"; message="Превышен лимит времени 5 секунд".into(); break; }
        };
        if !output.status.success() {
            final_status = if output.stderr.windows(12).any(|w| w == b"SyntaxError:") { "compile_error" } else { "runtime_error" };
            message = String::from_utf8_lossy(&output.stderr).chars().take(500).collect();
            break;
        }
        if output.stdout.len() > 16_384 { final_status="runtime_error"; message="Слишком большой вывод".into(); break; }
        let actual = String::from_utf8_lossy(&output.stdout).trim().to_string();
        if actual != test.output.trim() { final_status="wrong_answer"; message="Ответ не совпал с ожидаемым на одном из тестов".into(); break; }
        passed += 1;
    }
    let _ = request.submission_id;
    Ok(Json(JudgeResponse { status:final_status.into(), message, passed, total:tests.len() }))
}
fn internal<E: std::fmt::Display>(error: E) -> (StatusCode, Json<serde_json::Value>) { (StatusCode::INTERNAL_SERVER_ERROR, Json(serde_json::json!({"error":error.to_string()}))) }
#[tokio::main] async fn main() {
    let image = std::env::var("RUNNER_IMAGE").unwrap_or_else(|_| "python:3.12-alpine".into());
    let state = Arc::new(AppState { runner_image:image });
    let app = Router::new().route("/healthz", get(health)).route("/judge", post(judge)).with_state(state);
    let listener = tokio::net::TcpListener::bind("0.0.0.0:8090").await.expect("bind judge listener");
    axum::serve(listener, app).await.expect("serve judge API");
}
