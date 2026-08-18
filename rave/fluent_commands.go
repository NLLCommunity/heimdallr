package rave

import (
	"math"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/omit"
)

const (
	discordMaxCommandOptions     = 25
	discordMaxCommandChoices     = 25
	discordMaxStringOptionLength = 6000
	discordMaxChoiceNameLength   = 100
	discordMaxStringChoiceLength = 100
	discordMaxIntegerChoiceValue = 1<<53 - 1
	discordMinIntegerChoiceValue = -discordMaxIntegerChoiceValue
	discordMaxNumberChoiceValue  = 1 << 53
	discordMinNumberChoiceValue  = -discordMaxNumberChoiceValue
)

type SlashCommandBuilder struct {
	definitionBase[SlashCommandBuilder]
	commandMetadataBase[SlashCommandBuilder]
	options      []CommandOption
	handler      handler.CommandHandler
	slashHandler handler.SlashCommandHandler
}

type commandMetadataBase[T any] struct {
	self                     *T
	defaultMemberPermissions *discord.Permissions
	integrationTypes         []discord.ApplicationIntegrationType
	contexts                 []discord.InteractionContextType
	nsfw                     *bool
}

func newCommandMetadataBase[T any](self *T) commandMetadataBase[T] {
	return commandMetadataBase[T]{self: self}
}

func rejectMixedHandlerStyles(configuring, otherConfigured bool, commandKind string) {
	if configuring && otherConfigured {
		panic("Discord " + commandKind + " cannot mix generic and typed handlers")
	}
}

type CommandOption interface {
	build() discord.ApplicationCommandOption
	definitionName() string
}

func (s *SlashCommandBuilder) AddOptions(options ...CommandOption) *SlashCommandBuilder {
	s.options = append(s.options, options...)
	return s
}

func Slash(name, description string) *SlashCommandBuilder {
	s := &SlashCommandBuilder{}
	s.definitionBase = newDefinitionBase(s, name, description)
	s.commandMetadataBase = newCommandMetadataBase(s)
	return s
}

func (c *commandMetadataBase[T]) WithDefaultMemberPermissions(permissions discord.Permissions) *T {
	c.defaultMemberPermissions = &permissions
	return c.self
}

func (c *commandMetadataBase[T]) WithIntegrationTypes(types []discord.ApplicationIntegrationType) *T {
	c.integrationTypes = types
	return c.self
}

func (c *commandMetadataBase[T]) AddIntegrationTypes(integrationTypes ...discord.ApplicationIntegrationType) *T {
	c.integrationTypes = append(c.integrationTypes, integrationTypes...)
	return c.self
}

func (c *commandMetadataBase[T]) WithContexts(contexts []discord.InteractionContextType) *T {
	c.contexts = contexts
	return c.self
}

func (c *commandMetadataBase[T]) AddContexts(contexts ...discord.InteractionContextType) *T {
	c.contexts = append(c.contexts, contexts...)
	return c.self
}

func (c *commandMetadataBase[T]) WithNSFW(nsfw bool) *T {
	c.nsfw = &nsfw
	return c.self
}

func (s *SlashCommandBuilder) Handle(h handler.CommandHandler) *SlashCommandBuilder {
	rejectMixedHandlerStyles(h != nil, s.slashHandler != nil, "slash command")
	s.handler = h
	return s
}

func (s *SlashCommandBuilder) HandleSlash(h handler.SlashCommandHandler) *SlashCommandBuilder {
	rejectMixedHandlerStyles(h != nil, s.handler != nil, "slash command")
	s.slashHandler = h
	return s
}

func (s *SlashCommandBuilder) Build() discord.ApplicationCommandCreate {
	validateCommandDefinition(
		s.name,
		s.nameLocalizations,
		s.description,
		s.descriptionLocalizations,
	)
	validateCommandOptions(s.options)

	options := make([]discord.ApplicationCommandOption, len(s.options))
	for i, option := range s.options {
		options[i] = option.build()
	}

	return discord.SlashCommandCreate{
		Name:                     s.name,
		NameLocalizations:        s.nameLocalizations,
		Description:              s.description,
		DescriptionLocalizations: s.descriptionLocalizations,
		Options:                  options,
		DefaultMemberPermissions: omit.New(s.defaultMemberPermissions),
		IntegrationTypes:         s.integrationTypes,
		Contexts:                 s.contexts,
		NSFW:                     s.nsfw,
	}
}

// //////////////////////////////////////////////
type definitionBase[T any] struct {
	self                     *T
	name                     string
	nameLocalizations        map[discord.Locale]string
	description              string
	descriptionLocalizations map[discord.Locale]string
}

func newDefinitionBase[T any](self *T, name, description string) definitionBase[T] {
	return definitionBase[T]{
		self:        self,
		name:        name,
		description: description,
	}
}

func (o *definitionBase[T]) WithNameLocalizations(localizations map[discord.Locale]string) *T {
	o.nameLocalizations = localizations
	return o.self
}

func (o *definitionBase[T]) WithDescriptionLocalizations(localizations map[discord.Locale]string) *T {
	o.descriptionLocalizations = localizations
	return o.self
}

func (o *definitionBase[T]) AddNameLocalization(locale discord.Locale, name string) *T {
	if o.nameLocalizations == nil {
		o.nameLocalizations = make(map[discord.Locale]string)
	}
	o.nameLocalizations[locale] = name
	return o.self
}

func (o *definitionBase[T]) AddDescriptionLocalization(locale discord.Locale, description string) *T {
	if o.descriptionLocalizations == nil {
		o.descriptionLocalizations = make(map[discord.Locale]string)
	}
	o.descriptionLocalizations[locale] = description
	return o.self
}

func (o *definitionBase[T]) definitionName() string {
	return o.name
}

func (o *definitionBase[T]) validateBase(name string) {
	if !validDiscordName(o.name) {
		panic("invalid Discord " + name + " name: " + o.name)
	}

	if !validDiscordDescription(o.description) {
		panic("invalid Discord command option description: " + o.description)
	}

	for _, nameLoc := range o.nameLocalizations {
		if !validDiscordName(nameLoc) {
			panic("invalid Discord command option name localization: " + nameLoc)
		}
	}

	for _, descLoc := range o.descriptionLocalizations {
		if !validDiscordDescription(descLoc) {
			panic("invalid Discord command option description localization: " + descLoc)
		}
	}
}

type optionBase[T any] struct {
	definitionBase[T]
	required bool
}

func newOptionBase[T any](self *T, name, description string) optionBase[T] {
	return optionBase[T]{
		definitionBase: newDefinitionBase(self, name, description),
	}
}

func (o *optionBase[T]) WithRequired(required bool) *T {
	o.required = required
	return o.self
}

func (o *optionBase[T]) optionRequired() bool {
	return o.required
}

type optionSubCommand struct {
	definitionBase[optionSubCommand]
	options      []CommandOption
	handler      handler.CommandHandler
	slashHandler handler.SlashCommandHandler
}

func (s *optionSubCommand) AddOptions(options ...CommandOption) *optionSubCommand {
	s.options = append(s.options, options...)
	return s
}

func (s *optionSubCommand) Handle(h handler.CommandHandler) *optionSubCommand {
	rejectMixedHandlerStyles(h != nil, s.slashHandler != nil, "slash subcommand")
	s.handler = h
	return s
}

func (s *optionSubCommand) HandleSlash(h handler.SlashCommandHandler) *optionSubCommand {
	rejectMixedHandlerStyles(h != nil, s.handler != nil, "slash subcommand")
	s.slashHandler = h
	return s
}

func (s *optionSubCommand) build() discord.ApplicationCommandOption {
	s.validateBase("option")
	if hasNestedOptions(s.options) {
		panic("Discord subcommands cannot contain nested subcommands: " + s.name)
	}
	validateCommandOptions(s.options)

	options := make([]discord.ApplicationCommandOption, len(s.options))
	for i, option := range s.options {
		options[i] = option.build()
	}

	return discord.ApplicationCommandOptionSubCommand{
		Name:                     s.name,
		NameLocalizations:        s.nameLocalizations,
		Description:              s.description,
		DescriptionLocalizations: s.descriptionLocalizations,
		Options:                  options,
	}
}

func SubCommand(name, description string) *optionSubCommand {
	o := &optionSubCommand{}
	o.definitionBase = newDefinitionBase(o, name, description)
	return o
}

type optionSubCommandGroup struct {
	definitionBase[optionSubCommandGroup]
	options []*optionSubCommand
}

func (g *optionSubCommandGroup) AddOptions(options ...*optionSubCommand) *optionSubCommandGroup {
	g.options = append(g.options, options...)
	return g
}

func (g *optionSubCommandGroup) build() discord.ApplicationCommandOption {
	g.validateBase("option")
	validateCommandOptions(g.commandOptions())

	subCommands := make([]discord.ApplicationCommandOptionSubCommand, len(g.options))
	for i, option := range g.options {
		subCommands[i] = option.build().(discord.ApplicationCommandOptionSubCommand)
	}

	return discord.ApplicationCommandOptionSubCommandGroup{
		Name:                     g.name,
		NameLocalizations:        g.nameLocalizations,
		Description:              g.description,
		DescriptionLocalizations: g.descriptionLocalizations,
		Options:                  subCommands,
	}
}

func (g *optionSubCommandGroup) commandOptions() []CommandOption {
	options := make([]CommandOption, len(g.options))
	for i, option := range g.options {
		options[i] = option
	}
	return options
}

func SubCommandGroup(name, description string) *optionSubCommandGroup {
	o := &optionSubCommandGroup{}
	o.definitionBase = newDefinitionBase(o, name, description)
	return o
}

type optionString struct {
	optionBase[optionString]
	choices             []discord.ApplicationCommandOptionChoiceString
	autocompleteHandler internalAutocompleteHandler
	minLength           *int
	maxLength           *int
}

func (s *optionString) WithChoices(choices []discord.ApplicationCommandOptionChoiceString) *optionString {
	s.choices = choices
	return s
}

func (s *optionString) AddChoice(name string, value string) *optionString {
	s.choices = append(s.choices, discord.ApplicationCommandOptionChoiceString{
		Name:  name,
		Value: value,
	})
	return s
}

func (s *optionString) Autocomplete(h AutocompleteHandler[string]) *optionString {
	if s.autocompleteHandler != nil {
		panic("autocomplete handler already configured for option: " + s.name)
	}
	s.autocompleteHandler = adaptAutocomplete(h, parseStringAutocomplete, convertStringChoice)
	return s
}

func (s *optionString) autocompleteBinding() (string, internalAutocompleteHandler, bool) {
	return s.name, s.autocompleteHandler, s.autocompleteHandler != nil
}

func (s *optionString) WithMinLength(min int) *optionString {
	s.minLength = &min
	return s
}

func (s *optionString) WithMaxLength(max int) *optionString {
	s.maxLength = &max
	return s
}

func (s *optionString) build() discord.ApplicationCommandOption {
	s.validateBase("option")

	if s.minLength != nil && (*s.minLength < 0 || *s.minLength > discordMaxStringOptionLength) {
		panic("invalid Discord command option min length: " + strconv.Itoa(*s.minLength))
	}

	if s.maxLength != nil && (*s.maxLength < 1 || *s.maxLength > discordMaxStringOptionLength) {
		panic("invalid Discord command option max length: " + strconv.Itoa(*s.maxLength))
	}

	if s.minLength != nil && s.maxLength != nil && *s.minLength > *s.maxLength {
		panic("invalid Discord command option min/max length: min length cannot be greater than max length")
	}

	if !validDiscordChoiceCount(len(s.choices)) {
		panic("invalid Discord command option choices: cannot have more than 25 choices")
	}
	validateDiscordStringChoices(s.choices)

	if len(s.choices) > 0 && s.autocompleteHandler != nil {
		panic("invalid Discord command option: cannot have both choices and autocomplete enabled")
	}

	return discord.ApplicationCommandOptionString{
		Name:                     s.name,
		NameLocalizations:        s.nameLocalizations,
		Description:              s.description,
		DescriptionLocalizations: s.descriptionLocalizations,
		Required:                 s.required,
		Choices:                  s.choices,
		Autocomplete:             s.autocompleteHandler != nil,
		MinLength:                s.minLength,
		MaxLength:                s.maxLength,
	}
}

func OptionString(name, description string) *optionString {
	o := &optionString{}
	o.optionBase = newOptionBase(o, name, description)
	return o
}

type optionInt struct {
	optionBase[optionInt]
	choices             []discord.ApplicationCommandOptionChoiceInt
	autocompleteHandler internalAutocompleteHandler
	minValue            *int
	maxValue            *int
}

func (i *optionInt) WithChoices(choices []discord.ApplicationCommandOptionChoiceInt) *optionInt {
	i.choices = choices
	return i
}

func (i *optionInt) AddChoice(name string, value int) *optionInt {
	i.choices = append(i.choices, discord.ApplicationCommandOptionChoiceInt{
		Name:  name,
		Value: value,
	})
	return i
}

func (i *optionInt) Autocomplete(h AutocompleteHandler[int]) *optionInt {
	if i.autocompleteHandler != nil {
		panic("autocomplete handler already configured for option: " + i.name)
	}
	i.autocompleteHandler = adaptAutocomplete(h, parseIntAutocomplete, convertIntChoice)
	return i
}

func (i *optionInt) autocompleteBinding() (string, internalAutocompleteHandler, bool) {
	return i.name, i.autocompleteHandler, i.autocompleteHandler != nil
}

func (i *optionInt) WithMinValue(min int) *optionInt {
	i.minValue = &min
	return i
}

func (i *optionInt) WithMaxValue(max int) *optionInt {
	i.maxValue = &max
	return i
}

func (i *optionInt) build() discord.ApplicationCommandOption {
	i.validateBase("option")

	if i.minValue != nil && i.maxValue != nil && *i.minValue > *i.maxValue {
		panic("invalid Discord command option min/max value: min value cannot be greater than max value")
	}
	if i.minValue != nil && !validDiscordIntegerChoiceValue(*i.minValue) {
		panic("invalid Discord command option min value")
	}
	if i.maxValue != nil && !validDiscordIntegerChoiceValue(*i.maxValue) {
		panic("invalid Discord command option max value")
	}

	if !validDiscordChoiceCount(len(i.choices)) {
		panic("invalid Discord command option choices: cannot have more than 25 choices")
	}
	validateDiscordIntegerChoices(i.choices)

	if len(i.choices) > 0 && i.autocompleteHandler != nil {
		panic("invalid Discord command option: cannot have both choices and autocomplete enabled")
	}

	return discord.ApplicationCommandOptionInt{
		Name:                     i.name,
		NameLocalizations:        i.nameLocalizations,
		Description:              i.description,
		DescriptionLocalizations: i.descriptionLocalizations,
		Required:                 i.required,
		Choices:                  i.choices,
		Autocomplete:             i.autocompleteHandler != nil,
		MinValue:                 i.minValue,
		MaxValue:                 i.maxValue,
	}
}

func OptionInt(name, description string) *optionInt {
	o := &optionInt{}
	o.optionBase = newOptionBase(o, name, description)
	return o
}

type optionBool struct {
	optionBase[optionBool]
}

func (b *optionBool) build() discord.ApplicationCommandOption {
	b.validateBase("option")

	return discord.ApplicationCommandOptionBool{
		Name:                     b.name,
		NameLocalizations:        b.nameLocalizations,
		Description:              b.description,
		DescriptionLocalizations: b.descriptionLocalizations,
		Required:                 b.required,
	}
}

func OptionBool(name, description string) *optionBool {
	o := &optionBool{}
	o.optionBase = newOptionBase(o, name, description)
	return o
}

type optionUser struct {
	optionBase[optionUser]
}

func (u *optionUser) build() discord.ApplicationCommandOption {
	u.validateBase("option")

	return discord.ApplicationCommandOptionUser{
		Name:                     u.name,
		NameLocalizations:        u.nameLocalizations,
		Description:              u.description,
		DescriptionLocalizations: u.descriptionLocalizations,
		Required:                 u.required,
	}
}

func OptionUser(name, description string) *optionUser {
	o := &optionUser{}
	o.optionBase = newOptionBase(o, name, description)
	return o
}

type optionChannel struct {
	optionBase[optionChannel]
	channelTypes []discord.ChannelType
}

func (c *optionChannel) WithChannelTypes(types []discord.ChannelType) *optionChannel {
	c.channelTypes = types
	return c
}

func (c *optionChannel) AddChannelTypes(channelTypes ...discord.ChannelType) *optionChannel {
	c.channelTypes = append(c.channelTypes, channelTypes...)
	return c
}

func (c *optionChannel) build() discord.ApplicationCommandOption {
	c.validateBase("option")

	return discord.ApplicationCommandOptionChannel{
		Name:                     c.name,
		NameLocalizations:        c.nameLocalizations,
		Description:              c.description,
		DescriptionLocalizations: c.descriptionLocalizations,
		Required:                 c.required,
		ChannelTypes:             c.channelTypes,
	}
}

func OptionChannel(name, description string) *optionChannel {
	o := &optionChannel{}
	o.optionBase = newOptionBase(o, name, description)
	return o
}

type optionRole struct {
	optionBase[optionRole]
}

func (r *optionRole) build() discord.ApplicationCommandOption {
	r.validateBase("option")

	return discord.ApplicationCommandOptionRole{
		Name:                     r.name,
		NameLocalizations:        r.nameLocalizations,
		Description:              r.description,
		DescriptionLocalizations: r.descriptionLocalizations,
		Required:                 r.required,
	}
}

func OptionRole(name, description string) *optionRole {
	o := &optionRole{}
	o.optionBase = newOptionBase(o, name, description)
	return o
}

type optionMentionable struct {
	optionBase[optionMentionable]
}

func (m *optionMentionable) build() discord.ApplicationCommandOption {
	m.validateBase("option")

	return discord.ApplicationCommandOptionMentionable{
		Name:                     m.name,
		NameLocalizations:        m.nameLocalizations,
		Description:              m.description,
		DescriptionLocalizations: m.descriptionLocalizations,
		Required:                 m.required,
	}
}

func OptionMentionable(name, description string) *optionMentionable {
	o := &optionMentionable{}
	o.optionBase = newOptionBase(o, name, description)
	return o
}

type optionFloat struct {
	optionBase[optionFloat]
	choices             []discord.ApplicationCommandOptionChoiceFloat
	autocompleteHandler internalAutocompleteHandler
	minValue            *float64
	maxValue            *float64
}

func (f *optionFloat) WithChoices(choices []discord.ApplicationCommandOptionChoiceFloat) *optionFloat {
	f.choices = choices
	return f
}

func (f *optionFloat) AddChoice(name string, value float64) *optionFloat {
	f.choices = append(f.choices, discord.ApplicationCommandOptionChoiceFloat{
		Name:  name,
		Value: value,
	})
	return f
}

func (f *optionFloat) Autocomplete(h AutocompleteHandler[float64]) *optionFloat {
	if f.autocompleteHandler != nil {
		panic("autocomplete handler already configured for option: " + f.name)
	}
	f.autocompleteHandler = adaptAutocomplete(h, parseFloatAutocomplete, convertFloatChoice)
	return f
}

func (f *optionFloat) autocompleteBinding() (string, internalAutocompleteHandler, bool) {
	return f.name, f.autocompleteHandler, f.autocompleteHandler != nil
}

func (f *optionFloat) WithMinValue(min float64) *optionFloat {
	f.minValue = &min
	return f
}

func (f *optionFloat) WithMaxValue(max float64) *optionFloat {
	f.maxValue = &max
	return f
}

func (f *optionFloat) build() discord.ApplicationCommandOption {
	f.validateBase("option")

	if f.minValue != nil && f.maxValue != nil && *f.minValue > *f.maxValue {
		panic("invalid Discord command option min/max value: min value cannot be greater than max value")
	}
	if f.minValue != nil && !validDiscordNumberChoiceValue(*f.minValue) {
		panic("invalid Discord command option min value")
	}
	if f.maxValue != nil && !validDiscordNumberChoiceValue(*f.maxValue) {
		panic("invalid Discord command option max value")
	}
	if !validDiscordChoiceCount(len(f.choices)) {
		panic("invalid Discord command option choices: cannot have more than 25 choices")
	}
	validateDiscordNumberChoices(f.choices)
	if len(f.choices) > 0 && f.autocompleteHandler != nil {
		panic("invalid Discord command option: cannot have both choices and autocomplete enabled")
	}

	return discord.ApplicationCommandOptionFloat{
		Name:                     f.name,
		NameLocalizations:        f.nameLocalizations,
		Description:              f.description,
		DescriptionLocalizations: f.descriptionLocalizations,
		Required:                 f.required,
		Choices:                  f.choices,
		Autocomplete:             f.autocompleteHandler != nil,
		MinValue:                 f.minValue,
		MaxValue:                 f.maxValue,
	}
}

func OptionFloat(name, description string) *optionFloat {
	o := &optionFloat{}
	o.optionBase = newOptionBase(o, name, description)
	return o
}

type optionAttachment struct {
	optionBase[optionAttachment]
}

func (a *optionAttachment) build() discord.ApplicationCommandOption {
	a.validateBase("option")

	return discord.ApplicationCommandOptionAttachment{
		Name:                     a.name,
		NameLocalizations:        a.nameLocalizations,
		Description:              a.description,
		DescriptionLocalizations: a.descriptionLocalizations,
		Required:                 a.required,
	}
}

func OptionAttachment(name, description string) *optionAttachment {
	o := &optionAttachment{}
	o.optionBase = newOptionBase(o, name, description)
	return o
}

func validDiscordChoiceCount(count int) bool {
	return count <= discordMaxCommandChoices
}

func validDiscordChoiceName(name string) bool {
	return validUTF8RuneLength(name, 1, discordMaxChoiceNameLength)
}

func validDiscordStringChoiceValue(value string) bool {
	return validUTF8RuneLength(value, 0, discordMaxStringChoiceLength)
}

func validDiscordIntegerChoiceValue(value int) bool {
	return value >= discordMinIntegerChoiceValue && value <= discordMaxIntegerChoiceValue
}

func validDiscordNumberChoiceValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) &&
		value >= discordMinNumberChoiceValue && value <= discordMaxNumberChoiceValue
}

func validUTF8RuneLength(value string, minLength, maxLength int) bool {
	if !utf8.ValidString(value) {
		return false
	}
	length := utf8.RuneCountInString(value)
	return length >= minLength && length <= maxLength
}

func validateDiscordChoiceNames(name string, localizations map[discord.Locale]string) {
	if !validDiscordChoiceName(name) {
		panic("invalid Discord command option choice name: " + name)
	}
	for _, localization := range localizations {
		if !validDiscordChoiceName(localization) {
			panic("invalid Discord command option choice name localization: " + localization)
		}
	}
}

func validateDiscordStringChoices(choices []discord.ApplicationCommandOptionChoiceString) {
	for _, choice := range choices {
		validateDiscordChoiceNames(choice.Name, choice.NameLocalizations)
		if !validDiscordStringChoiceValue(choice.Value) {
			panic("invalid Discord string command option choice value")
		}
	}
}

func validateDiscordIntegerChoices(choices []discord.ApplicationCommandOptionChoiceInt) {
	for _, choice := range choices {
		validateDiscordChoiceNames(choice.Name, choice.NameLocalizations)
		if !validDiscordIntegerChoiceValue(choice.Value) {
			panic("invalid Discord integer command option choice value")
		}
	}
}

func validateDiscordNumberChoices(choices []discord.ApplicationCommandOptionChoiceFloat) {
	for _, choice := range choices {
		validateDiscordChoiceNames(choice.Name, choice.NameLocalizations)
		if !validDiscordNumberChoiceValue(choice.Value) {
			panic("invalid Discord number command option choice value")
		}
	}
}

func validateCommandOptions(options []CommandOption) {
	if len(options) > discordMaxCommandOptions {
		panic("invalid Discord command options: cannot have more than 25 options")
	}

	names := make(map[string]struct{}, len(options))
	hasPrimitive := false
	hasNested := false
	optionalSeen := false

	for _, option := range options {
		name := option.definitionName()
		if _, exists := names[name]; exists {
			panic("duplicate Discord command option name: " + name)
		}
		names[name] = struct{}{}

		switch option.(type) {
		case *optionSubCommand, *optionSubCommandGroup:
			hasNested = true
		default:
			hasPrimitive = true
			requiredOption, ok := option.(interface{ optionRequired() bool })
			if !ok {
				panic("primitive Discord command option is missing requiredness metadata: " + name)
			}
			if requiredOption.optionRequired() {
				if optionalSeen {
					panic("required Discord command options must precede optional options")
				}
			} else {
				optionalSeen = true
			}
		}
	}

	if hasPrimitive && hasNested {
		panic("Discord command cannot mix primitive options and subcommands")
	}
}

func validateCommandDefinition(
	name string,
	nameLocalizations map[discord.Locale]string,
	description string,
	descriptionLocalizations map[discord.Locale]string,
) {
	if !validDiscordName(name) {
		panic("invalid Discord command name: " + name)
	}
	if !validDiscordDescription(description) {
		panic("invalid Discord command description: " + description)
	}
	for _, localizedName := range nameLocalizations {
		if !validDiscordName(localizedName) {
			panic("invalid Discord command name localization: " + localizedName)
		}
	}
	for _, localizedDescription := range descriptionLocalizations {
		if !validDiscordDescription(localizedDescription) {
			panic("invalid Discord command description localization: " + localizedDescription)
		}
	}
}

// validDiscordName validates a Discord command option name:
// - 1 to 32 characters
// - No rune with a distinct lowercase mapping
// - Letters, numbers, Devanagari/Thai script runes, hyphens, underscores, and apostrophes
func validDiscordName(name string) bool {
	if !validUTF8RuneLength(name, 1, 32) {
		return false
	}

	for _, r := range name {
		if unicode.ToLower(r) != r {
			return false
		}
		switch {
		case r == '-' || r == '_' || r == '\'':
		case unicode.IsLetter(r):
		case unicode.IsNumber(r):
		case unicode.In(r, unicode.Devanagari, unicode.Thai):
		default:
			return false
		}
	}
	return true
}

func validDiscordDescription(description string) bool {
	if !utf8.ValidString(description) {
		return false
	}
	length := utf8.RuneCountInString(description)
	if length < 1 || length > 100 {
		return false
	}

	// rune range covers Unicode edge cases (e.g., multi-byte UTF-8)
	for _, r := range description {
		switch {
		case unicode.IsPrint(r):
		default:
			return false // rejects non-printable characters
		}
	}
	return true
}
