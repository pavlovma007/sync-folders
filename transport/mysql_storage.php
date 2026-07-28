<?php
/**
 * mysql_storage.php — файловое хранилище в MySQL через PHP.
 *
 * Размещается на любом хостинге с PHP + MySQL.
 * База и таблица создаются автоматически.
 *
 * API:
 *   POST /mysql_storage.php          — загрузить файл (multipart: file + group)
 *   GET  /mysql_storage.php          — JSON-список файлов (?group= для фильтра)
 *   GET  /mysql_storage.php?file_name=X — скачать файл
 *
 * Конфигурация (через переменные окружения или константы ниже):
 *   MYSQL_HOST     — хост MySQL (по умолч. localhost)
 *   MYSQL_PORT     — порт (по умолч. 3306)
 *   MYSQL_DB       — имя БД
 *   MYSQL_USER     — пользователь
 *   MYSQL_PASS     — пароль
 *   STORAGE_AUTH   — Basic Auth "user:password" (опционально)
 */

// ─── Конфигурация MySQL ─────────────────────────────────────
$db_host = getenv('MYSQL_HOST') ?: 'localhost';
$db_port = getenv('MYSQL_PORT') ?: '3306';
$db_name = getenv('MYSQL_DB') ?: 'file_storage';
$db_user = getenv('MYSQL_USER') ?: 'root';
$db_pass = getenv('MYSQL_PASS') ?: '';
$auth_env = getenv('STORAGE_AUTH') ?: '';

// ─── Basic Auth ──────────────────────────────────────────────
if (!empty($auth_env)) {
    $valid = false;
    $header = $_SERVER['HTTP_AUTHORIZATION']
        ?? $_SERVER['REDIRECT_HTTP_AUTHORIZATION']
        ?? '';
    if (preg_match('/^Basic\s+(.+)$/i', $header, $m)) {
        $decoded = base64_decode($m[1]);
        if ($decoded === $auth_env) {
            $valid = true;
        }
    }
    if (!$valid) {
        header('WWW-Authenticate: Basic realm="MySQL Storage"');
        http_response_code(401);
        echo json_encode(['error' => 'Unauthorized']);
        exit;
    }
}

// ─── Подключение к MySQL ────────────────────────────────────
$dsn = "mysql:host={$db_host};port={$db_port};dbname={$db_name};charset=utf8mb4";
try {
    $pdo = new PDO($dsn, $db_user, $db_pass, [
        PDO::ATTR_ERRMODE            => PDO::ERRMODE_EXCEPTION,
        PDO::ATTR_DEFAULT_FETCH_MODE => PDO::FETCH_ASSOC,
    ]);
} catch (PDOException $e) {
    http_response_code(500);
    echo json_encode(['error' => 'DB connection failed: ' . $e->getMessage()]);
    exit;
}

// ─── Создание таблицы ───────────────────────────────────────
$pdo->exec("CREATE TABLE IF NOT EXISTS files (
    id INT AUTO_INCREMENT PRIMARY KEY,
    file_name VARCHAR(255) NOT NULL,
    file_data LONGBLOB NOT NULL,
    file_group VARCHAR(50) DEFAULT 'default_group',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_file_name (file_name),
    INDEX idx_file_group (file_group, file_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4");

$method = $_SERVER['REQUEST_METHOD'];

// ─── GET: список или скачивание ─────────────────────────────
if ($method === 'GET') {
    // Скачивание файла по имени
    if (!empty($_GET['file_name'])) {
        $stmt = $pdo->prepare("SELECT file_data, file_name FROM files WHERE file_name = ? ORDER BY id DESC LIMIT 1");
        $stmt->execute([$_GET['file_name']]);
        $row = $stmt->fetch();

        if (!$row) {
            http_response_code(404);
            echo json_encode(['error' => 'File not found']);
            exit;
        }

        header('Content-Type: application/octet-stream');
        header('Content-Disposition: attachment; filename="' . $row['file_name'] . '"');
        echo $row['file_data'];
        exit;
    }

    // Список файлов
    $group = $_GET['group'] ?? null;
    if ($group) {
        $stmt = $pdo->prepare("SELECT file_name, file_group, created_at, LENGTH(file_data) AS file_size FROM files WHERE file_group = ? ORDER BY created_at DESC");
        $stmt->execute([$group]);
    } else {
        $stmt = $pdo->query("SELECT file_name, file_group, created_at, LENGTH(file_data) AS file_size FROM files ORDER BY created_at DESC");
    }

    $files = [];
    while ($row = $stmt->fetch()) {
        $files[] = [
            'name'      => $row['file_name'],
            'group'     => $row['file_group'],
            'size'      => (int)$row['file_size'],
            'mod_time'  => strtotime($row['created_at']),
        ];
    }

    header('Content-Type: application/json');
    echo json_encode($files);
    exit;
}

// ─── POST: загрузка файла ───────────────────────────────────
if ($method === 'POST') {
    if (!isset($_FILES['file']) || $_FILES['file']['error'] !== UPLOAD_ERR_OK) {
        http_response_code(400);
        echo json_encode(['error' => 'No file uploaded or upload error']);
        exit;
    }

    $file_name = basename($_FILES['file']['name']);
    $file_data = file_get_contents($_FILES['file']['tmp_name']);
    $file_group = $_POST['group'] ?? 'default_group';

    // Ограничение: max 16MB для LONGBLOB через MySQL
    if (strlen($file_data) > 16 * 1024 * 1024) {
        http_response_code(413);
        echo json_encode(['error' => 'File too large (max 16MB)']);
        exit;
    }

    $stmt = $pdo->prepare("INSERT INTO files (file_name, file_data, file_group) VALUES (?, ?, ?)");
    $stmt->execute([$file_name, $file_data, $file_group]);

    header('Content-Type: application/json');
    echo json_encode([
        'status'    => 'ok',
        'filename'  => $file_name,
        'group'     => $file_group,
        'size'      => strlen($file_data),
    ]);
    exit;
}

// ─── Другие методы ──────────────────────────────────────────
http_response_code(405);
header('Content-Type: application/json');
echo json_encode(['error' => 'Method not allowed']);
