#pragma once
#include <string>
#include <map>
#include <stdexcept>
#include "../db/mysql_client.hpp"
#include <nlohmann/json.hpp>

namespace analytics {

using json = nlohmann::json;

// aggregate runs one of: count, sum, avg, min, max, group_by on a table column.
inline json aggregate(db::MySQLClient& db,
                      const std::string& database,
                      const std::string& table,
                      const std::string& operation,
                      const std::string& column) {
    if (!database.empty()) {
        db.exec("USE `" + database + "`");
    }

    json result;
    result["operation"] = operation;
    result["table"] = table;
    result["column"] = column;

    if (operation == "count") {
        auto rows = db.query("SELECT COUNT(*) AS cnt FROM `" + table + "`");
        result["value"] = rows.empty() ? 0 : std::stol(rows[0]["cnt"]);

    } else if (operation == "sum") {
        auto rows = db.query("SELECT SUM(`" + column + "`) AS s FROM `" + table + "`");
        result["value"] = rows.empty() ? 0.0 : std::stod(rows[0]["s"].empty() ? "0" : rows[0]["s"]);

    } else if (operation == "avg") {
        auto rows = db.query("SELECT AVG(`" + column + "`) AS a FROM `" + table + "`");
        result["value"] = rows.empty() ? 0.0 : std::stod(rows[0]["a"].empty() ? "0" : rows[0]["a"]);

    } else if (operation == "min") {
        auto rows = db.query("SELECT MIN(`" + column + "`) AS m FROM `" + table + "`");
        result["value"] = rows.empty() ? nullptr : json(rows[0]["m"]);

    } else if (operation == "max") {
        auto rows = db.query("SELECT MAX(`" + column + "`) AS m FROM `" + table + "`");
        result["value"] = rows.empty() ? nullptr : json(rows[0]["m"]);

    } else if (operation == "group_by") {
        auto rows = db.query(
            "SELECT `" + column + "` AS grp, COUNT(*) AS cnt "
            "FROM `" + table + "` GROUP BY `" + column + "` ORDER BY cnt DESC");
        json groups = json::array();
        for (auto& r : rows) {
            groups.push_back({{"group", r["grp"]}, {"count", std::stol(r["cnt"])}});
        }
        result["groups"] = groups;

    } else {
        throw std::runtime_error("unknown operation: " + operation +
            ". Supported: count, sum, avg, min, max, group_by");
    }

    return result;
}

} // namespace analytics