<?php
/**
 * php_storage.php — простое файловое хранилище через PHP.
 *
 * Размещается на любом PHP-хостинге, не требует БД или composer.
 *
 * API:
 *   GET  /php_storage.php           — JSON-список файлов
 *   POST /php_storage.php           — загрузка файла (multipart, поле "file")
 *   GET  /uploads/{filename}        — скачивание файла напрямую
 *
 * Безопасность:
 *   - Файлы сохраняются с уникальным суффиксом (hex), перезапись невозможна
 *   - Поддержка Basic Auth (опционально, через переменную окружения STORAGE_AUTH)
 *   - Имена файлов фильтруются: только буквы, цифры, точка, дефис, подчёркивание
 *
 * Пример установки auth:
 *   export STORAGE_AUTH="user:password"
 *
 * Конфигурация nginx:
 *   location /uploads/ {
 *       try_files $uri =404;
 *   }
 *   location /php_storage.php {
 *       fastcgi_pass unix:/var/run/php/php8.x-fpm.sock;
 *       include fastcgi_params;
 *   }
 */

// ─── Конфигурация ────────────────────────────────────────────
$upload_dir = __DIR__ . '/uploads';
$auth_env   = getenv('STORAGE_AUTH');   // "user:password" или пусто (без auth)

// ─── Basic Auth ──────────────────────────────────────────────
if (!empty($auth_env)) {
    $valid = false;
    if (!empty($_SERVER['HTTP_AUTHORIZATION'])) {
        $header = $_SERVER['HTTP_AUTHORIZATION'];
    } elseif (!empty($_SERVER['REDIRECT_HTTP_AUTHORIZATION'])) {
        $header = $_SERVER['REDIRECT_HTTP_AUTHORIZATION'];
    } else {
        $header = '';
    }
    if (preg_match('/^Basic\s+(.+)$/i', $header, $m)) {
        $decoded = base64_decode($m[1]);
        if ($decoded === $auth_env) {
            $valid = true;
        }
    }
    if (!$valid) {
        header('WWW-Authenticate: Basic realm="Storage"');
        http_response_code(401);
        echo json_encode(['error' => 'Unauthorized']);
        exit;
    }
}

$method = $_SERVER['REQUEST_METHOD'];

// ─── GET: список файлов ──────────────────────────────────────
if ($method === 'GET') {
    $files = [];
    if (is_dir($upload_dir)) {
        $dh = opendir($upload_dir);
        if ($dh) {
            while (($file = readdir($dh)) !== false) {
                if ($file === '.' || $file === '..') continue;
                $path = $upload_dir . '/' . $file;
                if (!is_file($path)) continue;
                $files[] = [
                    'name'     => $file,
                    'size'     => filesize($path),
                    'mod_time' => filemtime($path),
                ];
            }
            closedir($dh);
        }
    }
    header('Content-Type: application/json');
    echo json_encode($files);
    exit;
}

// ─── POST: загрузка файла ────────────────────────────────────
if ($method === 'POST') {
    if (!isset($_FILES['file']) || $_FILES['file']['error'] !== UPLOAD_ERR_OK) {
        http_response_code(400);
        echo json_encode(['error' => 'No file uploaded or upload error']);
        exit;
    }

    $original_name = basename($_FILES['file']['name']);
    // Фильтр имени: только безопасные символы
    $safe_name = preg_replace('/[^a-zA-Z0-9._-]/', '_', $original_name);
    if ($safe_name === '' || $safe_name === '.') {
        $safe_name = 'file';
    }

    if (!is_dir($upload_dir)) {
        mkdir($upload_dir, 0755, true);
    }

    // Уникальный суффикс — защита от перезаписи
    $suffix = bin2hex(random_bytes(4));
    $stored_name = $safe_name . '.' . $suffix;
    $dest = $upload_dir . '/' . $stored_name;

    if (move_uploaded_file($_FILES['file']['tmp_name'], $dest)) {
        header('Content-Type: application/json');
        echo json_encode([
            'status'   => 'ok',
            'filename' => $stored_name,
            'size'     => filesize($dest),
            'mod_time' => filemtime($dest),
        ]);
    } else {
        http_response_code(500);
        echo json_encode(['error' => 'Failed to save file']);
    }
    exit;
}

// ─── Другие методы ──────────────────────────────────────────
http_response_code(405);
header('Content-Type: application/json');
echo json_encode(['error' => 'Method not allowed']);
