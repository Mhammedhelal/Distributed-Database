#include "crow_all.h"
#include "db/mysql_client.hpp"
#include "auth/hmac.hpp"
#include "replication/receiver.hpp"
#include "analytics/analytics.hpp"
#include <nlohmann/json.hpp>
#include <cstdlib>
#include <iostream>
#include <memory>
#include <string>

// Resolve include paths for headers in subdirs

using json = nlohmann::json;

// env_or returns the environment variable value or a default.
static std::string env_or(const char* key, const char* def) {
    const char* v = std::getenv(key);
    return v ? v : def;
}

int main() {
    // ── Config from environment ───────────────────────────────────────────────
    std::string mysql_host = env_or("MYSQL_HOST", "mysql-worker1");
    int         mysql_port = std::stoi(env_or("MYSQL_PORT", "3306"));
    std::string mysql_user = env_or("MYSQL_USER", "root");
    std::string mysql_pass = env_or("MYSQL_PASS", "rootpass");
    std::string mysql_db   = env_or("MYSQL_DB",   "distdb");
    std::string hmac_secret= env_or("HMAC_SECRET", "change-me-in-production");
    int         port       = std::stoi(env_or("PORT", "8081"));
    std::string node_id    = env_or("NODE_ID", "2");

    // ── MySQL connection ──────────────────────────────────────────────────────
    auto db = std::make_shared<db::MySQLClient>(
        mysql_host, mysql_port, mysql_user, mysql_pass, mysql_db);

    auth::TokenValidator validator(hmac_secret, 30);

    // ── Crow app ──────────────────────────────────────────────────────────────
    crow::SimpleApp app;

    // GET /health
    CROW_ROUTE(app, "/health")([]() {
        return crow::response(200, R"({"status":"ok","service":"worker-node1-cpp"})");
    });

    // POST /query  — SELECT only from external clients (no master token)
    CROW_ROUTE(app, "/query").methods("POST"_method)([&](const crow::request& req) {
        try {
            auto body = json::parse(req.body);
            std::string sql = body.value("sql", "");
            if (sql.empty())
                return crow::response(400, R"({"error":"sql field required"})");

            // Guard: only SELECT is allowed without master token
            std::string upper = sql;
            for (auto& c : upper) c = std::toupper(c);
            std::string trimmed = upper;
            size_t s = trimmed.find_first_not_of(" \t\n\r");
            if (s != std::string::npos) trimmed = trimmed.substr(s);

            bool has_token = !req.get_header_value("X-Master-Token").empty() &&
                             validator.validate(req.get_header_value("X-Master-Token"));

            if (!has_token) {
                if (trimmed.rfind("SELECT", 0) != 0) {
                    return crow::response(403,
                        R"({"error":"slaves accept INSERT/UPDATE/DELETE only from master"})");
                }
            }

            // Run query
            if (trimmed.rfind("SELECT", 0) == 0) {
                auto rows = db->query(sql);
                json resp = json::array();
                for (auto& r : rows) resp.push_back(r);
                return crow::response(200, resp.dump());
            } else {
                db->exec(sql);
                json resp = {{"affected_rows", (uint64_t)db->affectedRows()}};
                return crow::response(200, resp.dump());
            }
        } catch (const std::exception& e) {
            return crow::response(500, json({{"error", e.what()}}).dump());
        }
    });

    // POST /replication/apply  — master token required
    CROW_ROUTE(app, "/replication/apply").methods("POST"_method)(
        [&](const crow::request& req) {
        std::string tok = req.get_header_value("X-Master-Token");
        if (tok.empty() || !validator.validate(tok)) {
            return crow::response(403, R"({"error":"invalid or missing master token"})");
        }
        try {
            auto entry = json::parse(req.body);
            auto [ok, err] = replication::applyEntry(*db, entry);
            if (!ok)
                return crow::response(500, json({{"error", err}}).dump());

            uint64_t seq = entry.value("seq", (uint64_t)0);
            json resp = {{"ok", true}, {"node_id", node_id}, {"seq", seq}};
            return crow::response(200, resp.dump());
        } catch (const std::exception& e) {
            return crow::response(500, json({{"error", e.what()}}).dump());
        }
    });

    // POST /analytics  — master token required (special C++ bonus endpoint)
    CROW_ROUTE(app, "/analytics").methods("POST"_method)(
        [&](const crow::request& req) {
        std::string tok = req.get_header_value("X-Master-Token");
        if (tok.empty() || !validator.validate(tok)) {
            return crow::response(403, R"({"error":"invalid or missing master token"})");
        }
        try {
            auto body = json::parse(req.body);
            std::string database  = body.value("database", "");
            std::string table     = body.value("table", "");
            std::string operation = body.value("operation", "count");
            std::string column    = body.value("column", "*");

            auto result = analytics::aggregate(*db, database, table, operation, column);
            return crow::response(200, result.dump());
        } catch (const std::exception& e) {
            return crow::response(500, json({{"error", e.what()}}).dump());
        }
    });

    // POST /internal/heartbeat  — register self with master
    // (Workers send heartbeats; master calls this pattern on workers too)
    CROW_ROUTE(app, "/internal/catchup").methods("POST"_method)(
        [&](const crow::request& req) {
        std::string tok = req.get_header_value("X-Master-Token");
        if (tok.empty() || !validator.validate(tok)) {
            return crow::response(403, R"({"error":"invalid or missing master token"})");
        }
        try {
            auto body = json::parse(req.body);
            auto entries = body["entries"];
            for (auto& entry : entries) {
                auto [ok, err] = replication::applyEntry(*db, entry);
                if (!ok) {
                    return crow::response(500, json({{"error", "catch-up failed at seq " +
                        std::to_string(entry.value("seq", 0)) + ": " + err}}).dump());
                }
            }
            return crow::response(200, R"({"ok":true})");
        } catch (const std::exception& e) {
            return crow::response(500, json({{"error", e.what()}}).dump());
        }
    });

    std::cout << "worker-node1 (C++) listening on :" << port << std::endl;
    app.port(port).multithreaded().run();
    return 0;
}