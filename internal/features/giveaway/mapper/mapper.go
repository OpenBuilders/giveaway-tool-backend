package mapper

import (
	"context"
	"fmt"
	"sort"
	"time"

	channelservice "giveaway-tool-backend/internal/features/channel/service"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"giveaway-tool-backend/internal/features/giveaway/repository"
)

// ToGiveawayResponse maps Giveaway model to GiveawayResponse DTO
func ToGiveawayResponse(
	ctx context.Context,
	giveaway *models.Giveaway,
	repo repository.GiveawayRepository,
	channelService channelservice.ChannelService,
	debug bool,
	logger func(msg string, args ...interface{}),
) (*models.GiveawayResponse, error) {
	if debug {
		logger("[DEBUG] Converting giveaway %s to response", giveaway.ID)
	}

	participantsCount, err := repo.GetParticipantsCount(ctx, giveaway.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants count: %w", err)
	}

	uniquePrizes := make(map[string]models.PrizePlace)
	for _, prize := range giveaway.Prizes {
		prizeKey := fmt.Sprintf("%s_%s", prize.PrizeType, prize.PrizeID)
		if _, exists := uniquePrizes[prizeKey]; !exists {
			prizeCopy := prize
			prizeCopy.Place = "all"
			uniquePrizes[prizeKey] = prizeCopy
		}
	}
	prizes := make([]models.PrizePlace, 0, len(uniquePrizes))
	for _, prize := range uniquePrizes {
		prizes = append(prizes, prize)
	}

	reqWithInfo := make([]models.RequirementWithChannelInfo, 0, len(giveaway.Requirements))
	for _, req := range giveaway.Requirements {
		username := req.Username
		if username != "" && username[0] == '@' {
			username = username[1:]
		}
		title := ""
		if info, err := req.GetChatIDInfo(); err == nil && info.IsNumeric {
			title, _ = channelService.GetChannelTitle(ctx, info.NumericID)
		}
		avatarURL := "https://t.me/i/userpic/160/" + username + ".jpg"
		channelURL := "https://t.me/" + username
		reqWithInfo = append(reqWithInfo, models.RequirementWithChannelInfo{
			Type:        req.Type,
			Username:    req.Username,
			Description: req.Description,
			ChannelInfo: models.ChannelInfo{
				Title:      title,
				Username:   username,
				AvatarURL:  avatarURL,
				ChannelURL: channelURL,
			},
		})
	}

	response := &models.GiveawayResponse{
		ID:                giveaway.ID,
		CreatorID:         giveaway.CreatorID,
		Title:             giveaway.Title,
		Description:       giveaway.Description,
		StartedAt:         giveaway.StartedAt,
		EndsAt:            giveaway.StartedAt.Add(time.Duration(giveaway.Duration) * time.Second),
		MaxParticipants:   giveaway.MaxParticipants,
		WinnersCount:      giveaway.WinnersCount,
		Status:            giveaway.Status,
		CreatedAt:         giveaway.CreatedAt,
		UpdatedAt:         giveaway.UpdatedAt,
		ParticipantsCount: participantsCount,
		CanEdit:           giveaway.IsEditable(),
		UserRole:          "user",
		Prizes:            prizes,
		Requirements:      reqWithInfo,
		AutoDistribute:    giveaway.AutoDistribute,
		AllowTickets:      giveaway.AllowTickets,
		MsgID:             giveaway.MsgID,
		Sponsors:          giveaway.Sponsors,
	}

	if giveaway.Status == models.GiveawayStatusCompleted || giveaway.Status == models.GiveawayStatusHistory || giveaway.Status == models.GiveawayStatusProcessing {
		winners, err := repo.GetWinners(ctx, giveaway.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get winners: %w", err)
		}
		sort.Slice(winners, func(i, j int) bool {
			return winners[i].Place < winners[j].Place
		})
		response.Winners = winners
	}

	if response.Sponsors == nil {
		response.Sponsors = make([]models.ChannelInfo, 0)
	}

	if debug {
		logger("[DEBUG] Successfully converted giveaway %s to response", giveaway.ID)
	}

	return response, nil
}

// ToDetailedResponses maps a slice of Giveaways to a slice of GiveawayDetailedResponse
func ToDetailedResponses(
	ctx context.Context,
	giveaways []*models.Giveaway,
	repo repository.GiveawayRepository,
	userID int64,
	getPrizeStatus func(ctx context.Context, giveawayID, prizeID string) string,
	getPrizeReceivedTime func(ctx context.Context, giveawayID string, userID int64) time.Time,
	logger func(msg string, args ...interface{}),
) ([]*models.GiveawayDetailedResponse, error) {
	responses := make([]*models.GiveawayDetailedResponse, len(giveaways))
	for i, giveaway := range giveaways {
		response, err := ToDetailedResponse(ctx, giveaway, repo, userID, getPrizeStatus, getPrizeReceivedTime, logger)
		if err != nil {
			return nil, err
		}
		responses[i] = response
	}
	return responses, nil
}

// ToDetailedResponse maps a Giveaway to GiveawayDetailedResponse
func ToDetailedResponse(
	ctx context.Context,
	giveaway *models.Giveaway,
	repo repository.GiveawayRepository,
	userID int64,
	getPrizeStatus func(ctx context.Context, giveawayID, prizeID string) string,
	getPrizeReceivedTime func(ctx context.Context, giveawayID string, userID int64) time.Time,
	logger func(msg string, args ...interface{}),
) (*models.GiveawayDetailedResponse, error) {
	participantsCount, err := repo.GetParticipantsCount(ctx, giveaway.ID)
	if err != nil {
		return nil, err
	}
	creator, err := repo.GetCreator(ctx, giveaway.CreatorID)
	if err != nil {
		return nil, err
	}
	uniquePrizes := make(map[string]models.PrizeDetail)
	for _, prize := range giveaway.Prizes {
		prizeInfo, err := repo.GetPrize(ctx, prize.PrizeID)
		if err != nil {
			return nil, err
		}
		prizeKey := string(prizeInfo.Type) + "_" + prizeInfo.Name
		if _, exists := uniquePrizes[prizeKey]; !exists {
			uniquePrizes[prizeKey] = models.PrizeDetail{
				Type:        prizeInfo.Type,
				Name:        prizeInfo.Name,
				Description: prizeInfo.Description,
				IsInternal:  prizeInfo.IsInternal,
				Status:      getPrizeStatus(ctx, giveaway.ID, prize.PrizeID),
			}
		}
	}
	prizes := make([]models.PrizeDetail, 0, len(uniquePrizes))
	for _, prize := range uniquePrizes {
		prizes = append(prizes, prize)
	}
	userRole := "viewer"
	if giveaway.CreatorID == userID {
		userRole = "owner"
	} else {
		isParticipant, err := repo.IsParticipant(ctx, giveaway.ID, userID)
		if err != nil {
			return nil, err
		}
		if isParticipant {
			userRole = "participant"
		}
	}
	userTickets := 0
	totalTickets := 0
	if giveaway.AllowTickets {
		userTickets, err = repo.GetUserTickets(ctx, giveaway.ID, userID)
		if err != nil {
			return nil, err
		}
		totalTickets, err = repo.GetTotalTickets(ctx, giveaway.ID)
		if err != nil {
			return nil, err
		}
	}
	var winnerDetails []models.WinnerDetail
	if giveaway.Status == models.GiveawayStatusCompleted || giveaway.Status == models.GiveawayStatusHistory || giveaway.Status == models.GiveawayStatusProcessing {
		winners, err := repo.GetWinners(ctx, giveaway.ID)
		if err != nil {
			return nil, err
		}
		sort.Slice(winners, func(i, j int) bool {
			return winners[i].Place < winners[j].Place
		})
		winnerDetails = make([]models.WinnerDetail, len(winners))
		for i, winner := range winners {
			winnerUser, err := repo.GetUser(ctx, winner.UserID)
			if err != nil {
				return nil, err
			}
			prizeInfo, err := repo.GetPrize(ctx, giveaway.Prizes[winner.Place-1].PrizeID)
			if err != nil {
				return nil, err
			}
			winnerDetails[i] = models.WinnerDetail{
				UserID:   winner.UserID,
				Username: winnerUser.Username,
				Place:    winner.Place,
				Prize: models.PrizeDetail{
					Type:        prizeInfo.Type,
					Name:        prizeInfo.Name,
					Description: prizeInfo.Description,
					IsInternal:  prizeInfo.IsInternal,
					Status:      getPrizeStatus(ctx, giveaway.ID, giveaway.Prizes[winner.Place-1].PrizeID),
				},
				ReceivedAt: getPrizeReceivedTime(ctx, giveaway.ID, winner.UserID),
			}
		}
	}
	return &models.GiveawayDetailedResponse{
		ID:                giveaway.ID,
		CreatorID:         giveaway.CreatorID,
		CreatorUsername:   creator.Username,
		Title:             giveaway.Title,
		Description:       giveaway.Description,
		StartedAt:         giveaway.StartedAt,
		EndsAt:            giveaway.StartedAt.Add(time.Duration(giveaway.Duration) * time.Second),
		Duration:          giveaway.Duration,
		MaxParticipants:   giveaway.MaxParticipants,
		ParticipantsCount: participantsCount,
		WinnersCount:      giveaway.WinnersCount,
		Status:            giveaway.Status,
		CreatedAt:         giveaway.CreatedAt,
		UpdatedAt:         giveaway.UpdatedAt,
		Winners:           winnerDetails,
		Prizes:            prizes,
		UserRole:          userRole,
		UserTickets:       userTickets,
		TotalTickets:      totalTickets,
	}, nil
}
