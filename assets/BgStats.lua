-- BgStats v1.0.0
-- Captures end-of-BG scoreboard data and saves to SavedVariables for the companion app to upload.

local ADDON_VERSION = "1.0.0"

BgStatsDB = BgStatsDB or {}

-- ---------------------------------------------------------------------------
-- Encoding helpers
-- ---------------------------------------------------------------------------

local b64chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'

local function BgStats_Base64Encode(data)
    local result = {}
    for i = 1, #data, 3 do
        local a, b, c = data:byte(i, i + 2)
        b = b or 0
        c = c or 0
        local n = a * 65536 + b * 256 + c
        result[#result + 1] = b64chars:sub(math.floor(n / 262144) % 64 + 1, math.floor(n / 262144) % 64 + 1)
        result[#result + 1] = b64chars:sub(math.floor(n / 4096)   % 64 + 1, math.floor(n / 4096)   % 64 + 1)
        result[#result + 1] = b64chars:sub(math.floor(n / 64)     % 64 + 1, math.floor(n / 64)     % 64 + 1)
        result[#result + 1] = b64chars:sub(n % 64 + 1,                      n % 64 + 1)
    end
    local padding = (3 - #data % 3) % 3
    for i = 1, padding do result[#result - i + 1] = '=' end
    return table.concat(result)
end

-- DJB2 hash — matches the Go companion's computeChecksum implementation.
local function BgStats_Checksum(s)
    local hash = 5381
    for i = 1, #s do
        hash = (hash * 33 + s:byte(i)) % 4294967296
    end
    return string.format("%08x", hash)
end

-- Serialise a match table to a pipe-delimited string.
-- Uses -1 as a sentinel for BG-specific stats that don't apply to this BG type.
local function BgStats_Serialize(match)
    local scoreParts = {}
    for _, s in ipairs(match.scores) do
        local function n(v) return v ~= nil and v or -1 end
        scoreParts[#scoreParts + 1] = table.concat({
            s.characterName   or "",
            s.realm           or "",
            s.faction         or "",
            s.characterClass  or "",
            s.teamWon and "1" or "0",
            s.killingBlows    or 0,
            s.honorableKills  or 0,
            s.deaths          or 0,
            s.bonusHonor      or 0,
            n(s.flagCaptures),
            n(s.flagReturns),
            n(s.basesAssaulted),
            n(s.basesDefended),
            n(s.graveyardsAssaulted),
            n(s.graveyardsDefended),
            n(s.towersAssaulted),
            n(s.towersDefended),
        }, ",")
    end
    return table.concat({
        "bg:"     .. (match.battleground   or ""),
        "id:"     .. (match.instanceId     or 0),
        "start:"  .. (match.matchStart     or 0),
        "end:"    .. (match.matchEnd       or 0),
        "dur:"    .. (match.durationSeconds or 0),
        "win:"    .. (match.winningFaction  or ""),
        "ver:"    .. (match.addonVersion    or ""),
        "scores:" .. table.concat(scoreParts, ";"),
    }, "|")
end

local function initDB()
    if not BgStatsDB.matches then BgStatsDB.matches = {} end
    if not BgStatsDB.uploaded then BgStatsDB.uploaded = {} end
    if not BgStatsDB.inProgress then BgStatsDB.inProgress = {} end

    -- Prune matches that have already been uploaded to keep the file small.
    -- The companion writes to BgStatsDB.uploaded[matchKey], not m.uploaded.
    local pending = {}
    for _, m in ipairs(BgStatsDB.matches) do
        if not BgStatsDB.uploaded[m.key] then
            table.insert(pending, m)
        end
    end
    BgStatsDB.matches = pending
end


-- Zone name → BG type. Works regardless of instance ID or map ID quirks.
local BG_BY_ZONE = {
    ["Warsong Gulch"] = "WSG",
    ["Arathi Basin"]  = "AB",
    ["Alterac Valley"] = "AV",
}

local BG_STAT_INDICES = {
    WSG = {
        { key = "flagCaptures", index = 1 },
        { key = "flagReturns",  index = 2 },
    },
    AB = {
        { key = "basesAssaulted", index = 1 },
        { key = "basesDefended",  index = 2 },
    },
    AV = {
        { key = "graveyardsAssaulted", index = 1 },
        { key = "graveyardsDefended",  index = 2 },
        { key = "towersAssaulted",     index = 3 },
        { key = "towersDefended",      index = 4 },
    },
}

local frame = CreateFrame("Frame")
local matchStartTime  = nil
local currentBg       = nil
local currentInstanceId = nil
local captured        = false  -- guard against double-capture

frame:RegisterEvent("PLAYER_ENTERING_WORLD")
frame:RegisterEvent("ZONE_CHANGED_NEW_AREA")
frame:RegisterEvent("UPDATE_BATTLEFIELD_SCORE")

frame:SetScript("OnEvent", function(self, event)
    if event == "PLAYER_ENTERING_WORLD" or event == "ZONE_CHANGED_NEW_AREA" then
        BgStats_OnZoneChange()
    elseif event == "UPDATE_BATTLEFIELD_SCORE" then
        BgStats_OnScoreUpdate()
    end
end)

function BgStats_OnZoneChange()
    initDB()
    local zoneName = GetRealZoneText()
    local bg = BG_BY_ZONE[zoneName]

    if bg then
        if not currentBg then
            currentBg = bg
            currentInstanceId = select(8, GetInstanceInfo()) or 0
            -- Restore start time if this looks like the same BG (reload mid-match)
            if BgStatsDB.inProgress.bg == bg and BgStatsDB.inProgress.instanceId == currentInstanceId then
                matchStartTime = BgStatsDB.inProgress.matchStart
                DEFAULT_CHAT_FRAME:AddMessage("|cff00ff00[BgStats]|r Resuming " .. bg .. " match tracking.")
            else
                matchStartTime = time()
                BgStatsDB.inProgress = { bg = bg, instanceId = currentInstanceId, matchStart = matchStartTime }
                DEFAULT_CHAT_FRAME:AddMessage("|cff00ff00[BgStats]|r Tracking " .. bg .. " match.")
            end
            captured = false
        end
    else
        -- Left the BG zone
        currentBg = nil
        currentInstanceId = nil
        matchStartTime = nil
        captured = false
        BgStatsDB.inProgress = {}
    end
end

function BgStats_OnScoreUpdate()
    if not currentBg or captured then return end

    -- GetBattlefieldWinner() returns 0 (Horde), 1 (Alliance), or nil (in progress)
    local winner = GetBattlefieldWinner()
    if winner == nil then return end

    captured = true
    -- Small delay to ensure all score data is populated
    C_Timer.After(0.5, BgStats_CaptureScoreboard)
end

function BgStats_CaptureScoreboard()
    if not currentBg then return end

    -- Request a fresh data pull in case scores aren't ready yet
    RequestBattlefieldScoreData()

    local numScores = GetNumBattlefieldScores()
    if numScores == 0 then
        DEFAULT_CHAT_FRAME:AddMessage("|cffff4444[BgStats]|r Scoreboard empty, retrying in 1s...")
        C_Timer.After(1, BgStats_CaptureScoreboard)
        return
    end

    local matchEndTime  = time()
    local winnerFaction = GetBattlefieldWinner()
    if winnerFaction == nil then return end

    local winner = (winnerFaction == 0) and "HORDE" or "ALLIANCE"

    local matchKey = currentBg .. ":" .. (currentInstanceId or 0) .. ":" .. matchStartTime
    if BgStatsDB.uploaded[matchKey] then
        DEFAULT_CHAT_FRAME:AddMessage("|cffffff00[BgStats]|r Match already recorded.")
        return
    end

    local scores     = {}
    local playerRealm = GetRealmName()

for i = 1, numScores do
        local name, killingBlows, honorableKills, deaths, honorGained,
              faction, raceId, raceName, className, classToken, v11, v12 = GetBattlefieldScore(i)

        if name then
            local charName, realm = strsplit("-", name, 2)
            if not realm or realm == "" then realm = playerRealm end

            local factionStr = (faction == 1) and "ALLIANCE" or "HORDE"
            local teamWon    = (factionStr == winner)
            local entry = {
                characterName  = charName,
                realm          = realm,
                faction        = factionStr,
                characterClass = classToken,
                className      = className,
                race           = raceName or "",
                raceId         = raceId or 0,
                extraV11       = v11 or 0,
                extraV12       = v12 or 0,
                teamWon        = teamWon,
                killingBlows   = killingBlows or 0,
                honorableKills = honorableKills or 0,
                deaths         = deaths or 0,
                bonusHonor     = honorGained or 0,
            }

            BgStats_AttachBgStats(entry, i)
            table.insert(scores, entry)
        end
    end

    local match = {
        battleground    = currentBg,
        instanceId      = currentInstanceId or 0,
        matchStart      = matchStartTime,
        matchEnd        = matchEndTime,
        durationSeconds = matchEndTime - (matchStartTime or matchEndTime),
        winningFaction = winner,
        addonVersion   = ADDON_VERSION,
        scores         = scores,
        uploaded       = false,
    }

    local serialized = BgStats_Serialize(match)
    local checksum   = BgStats_Checksum(serialized)
    local encoded    = BgStats_Base64Encode(serialized .. "|chk:" .. checksum)
    table.insert(BgStatsDB.matches, { key = matchKey, data = encoded })
    DEFAULT_CHAT_FRAME:AddMessage(
        string.format("|cff00ff00[BgStats]|r %s match recorded (%d players). Type |cffffff00/reload|r to sync to the website now.",
            currentBg, #scores))

    currentBg         = nil
    currentInstanceId = nil
    matchStartTime    = nil
    BgStatsDB.inProgress = {}
end

function BgStats_AttachBgStats(entry, playerIndex)
    local statDefs = BG_STAT_INDICES[currentBg]
    if not statDefs then return end
    for _, def in ipairs(statDefs) do
        local value = GetBattlefieldStatData(playerIndex, def.index)
        entry[def.key] = value or 0
    end
end

SLASH_BGSTATS1 = "/bgstats"
SlashCmdList["BGSTATS"] = function()
    initDB()
    local total, pending = #BgStatsDB.matches, 0
    for _, m in ipairs(BgStatsDB.matches) do
        if not BgStatsDB.uploaded[m.key] then pending = pending + 1 end
    end
    DEFAULT_CHAT_FRAME:AddMessage(
        string.format("|cff00ff00[BgStats]|r %d matches recorded, %d pending upload.", total, pending))
    -- Show current tracking state
    if currentBg then
        DEFAULT_CHAT_FRAME:AddMessage("|cff00ff00[BgStats]|r Currently tracking: " .. currentBg)
    end
end
