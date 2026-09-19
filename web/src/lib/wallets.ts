// Donation wallets shown in the "Support the project" dialog.
//
// ⚠️ IMPORTANT: the addresses below are PLACEHOLDERS (well-known burn/test
// addresses). Replace them with the project's real addresses before deploying —
// they are compiled into the public web bundle and visible to everyone.
export type Wallet = {
	/** Stable id used as a list key and for the "copied" state. */
	id: string;
	/** Network / coin name (brand name, not translated). */
	name: string;
	/** Short ticker or network tag shown as a badge. */
	symbol: string;
	/** Wallet address copied to the clipboard. */
	address: string;
};

export const wallets: Wallet[] = [
	{
		id: 'btc',
		name: 'Bitcoin',
		symbol: 'BTC',
		address: 'bc1qke444m9h609ulz87gvkaj00v9x7pwxu9t443cz'
	},
	{
		id: 'eth',
		name: 'Ethereum',
		symbol: 'ETH',
		address: '0x0A662E57d51c435fd1ebDcB9DDbf0550d53605F3'
	},
	{
		id: 'usdt-trc20',
		name: 'USDT',
		symbol: 'Tron',
		address: 'TXHSyzj1uciawaeFyj4hfNGf4eU3C59vxu'
	}
];
