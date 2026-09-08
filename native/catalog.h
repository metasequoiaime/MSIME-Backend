#pragma once
#include <nlohmann/json.hpp>
#include <sqlite3.h>
#include <filesystem>
#include <memory>
#include <string>

inline nlohmann::json query_catalog(const nlohmann::json& request, const std::filesystem::path& resources) {
    using nlohmann::json;
    const auto kind = request.at("kind").get<std::string>();
    std::string table, value, category, parent;
    if (kind == "emoji") {table = "emoji"; value = "emoji"; category = "category"; parent = "''";}
    else if (kind == "kaomoji") {table = "kaomoji_catalog"; value = "kaomoji"; category = "'All'"; parent = "''";}
    else if (kind == "symbols") {table = "symbol_catalog"; value = "symbol"; category = "category"; parent = "parent_category";}
    else return {{"error", "invalid_request"}};
    const auto path = resources / "others.db";
    sqlite3* raw = nullptr;
    const int opened = sqlite3_open_v2(path.string().c_str(), &raw, SQLITE_OPEN_READONLY, nullptr);
    std::unique_ptr<sqlite3, decltype(&sqlite3_close)> db(raw, sqlite3_close);
    if (opened != SQLITE_OK) return {{"error", "resources_unavailable"}};
    sqlite3_busy_timeout(db.get(), 1000);
    const int offset = request.value("offset", 0), limit = request.value("limit", 50);
    if (offset < 0 || offset > 1000000 || limit < 1 || limit > 200) return {{"error", "invalid_request"}};
    const auto search = request.value("text", std::string()), filter = request.value("category", std::string());
    const auto sql = "SELECT " + value + "," + category + "," + parent + ",keywords FROM " + table +
        " WHERE (?1='' OR " + category + "=?1) AND (?2='' OR instr(lower(keywords),lower(?2))>0 OR instr(" + value +
        ",?2)>0) ORDER BY sort_order," + value + " LIMIT ?3 OFFSET ?4";
    sqlite3_stmt* stmt = nullptr;
    if (sqlite3_prepare_v2(db.get(), sql.c_str(), -1, &stmt, nullptr) != SQLITE_OK) return {{"error", "resources_unavailable"}};
    std::unique_ptr<sqlite3_stmt, decltype(&sqlite3_finalize)> statement(stmt, sqlite3_finalize);
    sqlite3_bind_text(stmt, 1, filter.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 2, search.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int(stmt, 3, limit + 1); sqlite3_bind_int(stmt, 4, offset);
    auto items = json::array();
    int code;
    while ((code = sqlite3_step(stmt)) == SQLITE_ROW) {
        auto str = [stmt](int col) {const auto* p = sqlite3_column_text(stmt, col); return p ? std::string(reinterpret_cast<const char*>(p)) : std::string();};
        items.push_back({{"text",str(0)}, {"category",str(1)}, {"parent_category",str(2)}, {"keywords",str(3)}});
    }
    if (code != SQLITE_DONE) return {{"error", "resources_unavailable"}};
    const bool more = items.size() > static_cast<std::size_t>(limit);
    if (more) items.erase(items.end()-1);
    statement.reset();
    const auto category_sql = "SELECT " + category + "," + parent + ",count(*) FROM " + table + " GROUP BY " + category + "," + parent + " ORDER BY min(sort_order)";
    if (sqlite3_prepare_v2(db.get(), category_sql.c_str(), -1, &stmt, nullptr) != SQLITE_OK) return {{"error", "resources_unavailable"}};
    statement.reset(stmt);
    auto categories = json::array();
    while ((code = sqlite3_step(stmt)) == SQLITE_ROW) {
        auto str = [stmt](int col) {const auto* p = sqlite3_column_text(stmt, col); return p ? std::string(reinterpret_cast<const char*>(p)) : std::string();};
        categories.push_back({{"name",str(0)}, {"parent",str(1)}, {"count",sqlite3_column_int(stmt,2)}});
    }
    if (code != SQLITE_DONE) return {{"error", "resources_unavailable"}};
    return {{"items",items}, {"categories",categories}, {"offset",offset}, {"has_more",more}};
}
