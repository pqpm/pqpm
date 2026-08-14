#!/usr/bin/perl
require './pqpm-lib.pl';
&ReadParse();

ui_print_header(undef, $text{'config_title'} || 'PQPM configuration', '');

if ($in{'save'}) {
	my $err = &write_user_config($in{'content'} // '');
	if ($err) {
		print &ui_alert_box(&html_escape($err), 'danger');
	} else {
		print &ui_alert_box($text{'saved'} || 'Saved ~/.pqpm.toml', 'success');
		if ($in{'reload'}) {
			my ($code, $out) = &pqpm_run('reload', '--all');
			if ($code != 0) {
				print &ui_alert_box(&html_escape($out || 'reload failed'), 'warn');
			} else {
				print &ui_alert_box(&html_escape($out || 'Reloaded services'), 'success');
			}
		}
	}
}

my ($path, $content) = &read_user_config();
print &ui_form_start('config.cgi', 'post');
print &ui_hidden('save', 1);
print '<p>', ($text{'editing'} || 'Editing'), ' <tt>', &html_escape($path), '</tt></p>';
print &ui_textarea('content', $content, 24, 80);
print '<p>', &ui_checkbox('reload', 1, $text{'reload_after'} || 'Reload all services after save', 1), '</p>';
print &ui_form_end([
	[ 'ok', $text{'save'} || 'Save' ],
]);
&ui_print_footer('index.cgi', $text{'back'} || 'Back');
